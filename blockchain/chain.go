// Package blockchain implements the on-chain layer for clawdamp.
// It provides a simple KV store on top of a local append-only ledger
// that can sync with other nodes over p2p. Track metadata, playlist
// ownership, tipping records, and stream proofs are recorded here.
package blockchain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// HashSize is the length of a SHA-256 hash in bytes.
const HashSize = 32

// Hash wraps a SHA-256 hash.
type Hash [HashSize]byte

// ZeroHash is the all-zero hash.
var ZeroHash Hash

// Account represents a balance entry on the clawdamp chain.
type Account struct {
	Name    string `json:"name"`
	Balance uint64 `json:"balance"`
	Nonce   uint64 `json:"nonce"`
}

// TrackRecord is on-chain track metadata.
type TrackRecord struct {
	CID          string    `json:"cid"`
	Title        string    `json:"title"`
	Artist       string    `json:"artist"`
	Uploader     []byte    `json:"uploader"`
	PlayCount    uint64    `json:"play_count"`
	TipTotal     uint64    `json:"tip_total"`
	RegisteredAt time.Time `json:"registered_at"`
}

// PlaylistRecord is an on-chain playlist owned by an agent.
type PlaylistRecord struct {
	ID        string    `json:"id"`
	Owner     []byte    `json:"owner"`
	Name      string    `json:"name"`
	TrackCIDs []string  `json:"track_cids"`
	CreatedAt time.Time `json:"created_at"`
}

// OpKind classifies a block operation.
type OpKind string

const (
	OpGenesis        OpKind = "genesis"
	OpTransfer       OpKind = "transfer"
	OpRegisterTrack  OpKind = "register_track"
	OpTipTrack       OpKind = "tip_track"
	OpCreatePlaylist OpKind = "create_playlist"
	OpStreamProof    OpKind = "stream_proof"
)

// Operation is a single state mutation in a block.
type Operation struct {
	Kind      OpKind          `json:"kind"`
	From      []byte          `json:"from,omitempty"`
	To        []byte          `json:"to,omitempty"`
	Amount    uint64          `json:"amount,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Nonce     uint64          `json:"nonce"`
	Signature []byte          `json:"signature"`
}

// Block is a container of ordered operations.
type Block struct {
	Index     uint64      `json:"index"`
	PrevHash  Hash        `json:"prev_hash"`
	Timestamp time.Time   `json:"timestamp"`
	Ops       []Operation `json:"ops"`
	Hash      Hash        `json:"hash"`
}

// ComputeHash hashes the block header.
func (b *Block) ComputeHash() Hash {
	h := sha256.New()
	fmt.Fprintf(h, "%d", b.Index)
	h.Write(b.PrevHash[:])
	fmt.Fprintf(h, "%d", b.Timestamp.UnixNano())
	for _, op := range b.Ops {
		enc, _ := json.Marshal(op)
		h.Write(enc)
	}
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

// Ledger is the append-only local chain state.
type Ledger struct {
	mu        sync.RWMutex
	Blocks    []*Block                   `json:"blocks"`
	Accounts  map[string]*Account        `json:"accounts"`
	Tracks    map[string]*TrackRecord    `json:"tracks"`
	Playlists map[string]*PlaylistRecord `json:"playlists"`
}

// NewLedger creates a fresh ledger with a genesis block.
func NewLedger(genesisKey ed25519.PublicKey) *Ledger {
	ledger := &Ledger{
		Accounts:  make(map[string]*Account),
		Tracks:    make(map[string]*TrackRecord),
		Playlists: make(map[string]*PlaylistRecord),
	}
	genHex := fmt.Sprintf("%x", genesisKey)
	ledger.Accounts[genHex] = &Account{Name: "genesis", Balance: 1_000_000_000, Nonce: 0}

	genesis := &Block{
		Index:     0,
		PrevHash:  ZeroHash,
		Timestamp: time.Now(),
		Ops:       []Operation{{Kind: OpGenesis}},
	}
	genesis.Hash = genesis.ComputeHash()
	ledger.Blocks = append(ledger.Blocks, genesis)
	return ledger
}

// AddBlock appends a validated block to the chain.
func (l *Ledger) AddBlock(b *Block) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	last := l.Blocks[len(l.Blocks)-1]
	if b.Index != last.Index+1 {
		return fmt.Errorf("blockchain: invalid index %d, expected %d", b.Index, last.Index+1)
	}
	if b.PrevHash != last.Hash {
		return fmt.Errorf("blockchain: prev hash mismatch")
	}

	for i, op := range b.Ops {
		if err := l.applyOp(&op); err != nil {
			return fmt.Errorf("blockchain: block %d op %d: %w", b.Index, i, err)
		}
	}

	b.Hash = b.ComputeHash()
	l.Blocks = append(l.Blocks, b)
	return nil
}

func (l *Ledger) applyOp(op *Operation) error {
	switch op.Kind {
	case OpTransfer:
		return l.applyTransfer(op)
	case OpRegisterTrack:
		return l.applyRegisterTrack(op)
	case OpTipTrack:
		return l.applyTipTrack(op)
	case OpCreatePlaylist:
		return l.applyCreatePlaylist(op)
	case OpStreamProof, OpGenesis:
		return nil
	default:
		return fmt.Errorf("unknown op kind %q", op.Kind)
	}
}

func (l *Ledger) applyTransfer(op *Operation) error {
	from := fmt.Sprintf("%x", op.From)
	to := fmt.Sprintf("%x", op.To)

	sender, ok := l.Accounts[from]
	if !ok {
		return fmt.Errorf("sender %s not found", from[:8])
	}
	if sender.Balance < op.Amount {
		return fmt.Errorf("insufficient balance: %d < %d", sender.Balance, op.Amount)
	}
	if op.Nonce != sender.Nonce+1 {
		return fmt.Errorf("invalid nonce %d, expected %d", op.Nonce, sender.Nonce+1)
	}

	sender.Balance -= op.Amount
	sender.Nonce = op.Nonce

	recv, ok := l.Accounts[to]
	if !ok {
		recv = &Account{Name: to[:8]}
		l.Accounts[to] = recv
	}
	recv.Balance += op.Amount
	return nil
}

func (l *Ledger) applyRegisterTrack(op *Operation) error {
	var track TrackRecord
	if err := json.Unmarshal(op.Data, &track); err != nil {
		return fmt.Errorf("invalid track data: %w", err)
	}
	if _, exists := l.Tracks[track.CID]; exists {
		return fmt.Errorf("track %s already registered", track.CID)
	}
	track.Uploader = op.From
	track.RegisteredAt = time.Now()
	l.Tracks[track.CID] = &track
	return nil
}

func (l *Ledger) applyTipTrack(op *Operation) error {
	var data struct {
		CID string `json:"cid"`
	}
	if err := json.Unmarshal(op.Data, &data); err != nil {
		return fmt.Errorf("invalid tip data: %w", err)
	}
	track, ok := l.Tracks[data.CID]
	if !ok {
		return fmt.Errorf("track %s not found", data.CID)
	}
	if err := l.applyTransfer(op); err != nil {
		return err
	}
	track.TipTotal += op.Amount
	track.PlayCount++
	return nil
}

func (l *Ledger) applyCreatePlaylist(op *Operation) error {
	var pl PlaylistRecord
	if err := json.Unmarshal(op.Data, &pl); err != nil {
		return fmt.Errorf("invalid playlist data: %w", err)
	}
	pl.Owner = op.From
	pl.CreatedAt = time.Now()
	l.Playlists[pl.ID] = &pl
	return nil
}

// GetBalance returns the balance for a public key.
func (l *Ledger) GetBalance(pub ed25519.PublicKey) uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	hex := fmt.Sprintf("%x", pub)
	if acct, ok := l.Accounts[hex]; ok {
		return acct.Balance
	}
	return 0
}

// GetTrack returns a track record by CID.
func (l *Ledger) GetTrack(cid string) *TrackRecord {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.Tracks[cid]
}

// GetPlaylist returns a playlist record.
func (l *Ledger) GetPlaylist(id string) *PlaylistRecord {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.Playlists[id]
}

// LastBlock returns the most recent block.
func (l *Ledger) LastBlock() *Block {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.Blocks) == 0 {
		return nil
	}
	return l.Blocks[len(l.Blocks)-1]
}

// Len returns the number of blocks.
func (l *Ledger) Len() uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return uint64(len(l.Blocks))
}

// NewOperation creates a signed operation.
func NewOperation(kind OpKind, from ed25519.PrivateKey, to ed25519.PublicKey, amount uint64, nonce uint64, data interface{}) (Operation, error) {
	op := Operation{
		Kind:   kind,
		From:   from.Public().(ed25519.PublicKey),
		To:     to,
		Amount: amount,
		Nonce:  nonce,
	}
	if data != nil {
		enc, err := json.Marshal(data)
		if err != nil {
			return op, fmt.Errorf("marshal data: %w", err)
		}
		op.Data = enc
	}
	sigBytes, _ := json.Marshal(struct {
		Kind   OpKind `json:"kind"`
		From   []byte `json:"from"`
		To     []byte `json:"to"`
		Amount uint64 `json:"amount"`
		Nonce  uint64 `json:"nonce"`
	}{op.Kind, op.From, op.To, op.Amount, op.Nonce})
	op.Signature = ed25519.Sign(from, sigBytes)
	return op, nil
}

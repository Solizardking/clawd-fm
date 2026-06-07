// Package radio is the core CLAWD FM engine.
// It orchestrates agents, the local ledger, Solana on-chain ops, and the p2p
// network to deliver the first Solana onchain terminal radio station.
package radio

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"

	"clawdamp/agent"
	"clawdamp/blockchain"
	"clawdamp/p2p"
	solanaclient "clawdamp/solana"
)

// Station manages the CLAWD FM radio station on a single node.
type Station struct {
	mu       sync.RWMutex
	Name     string
	Identity *agent.Identity
	PrivKey  ed25519.PrivateKey

	// Core subsystems.
	Ledger  *blockchain.Ledger
	Network *p2p.Network
	Agents  map[string]*agent.Agent
	Solana  *solanaclient.Client // nil when running without Solana

	// Radio state.
	NowPlaying *Track
	Listeners  int
	Queue      []*Track
	Streaming  bool
	CurrentDJ  *agent.Identity

	// Outbound audio chunk channel.
	streamOut chan *p2p.StreamChunk

	// Station event bus.
	events chan StationEvent

	// Lifecycle.
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// Track represents a track in the CLAWD FM queue.
type Track struct {
	CID          string          `json:"cid"`
	Title        string          `json:"title"`
	Artist       string          `json:"artist"`
	Duration     time.Duration   `json:"duration"`
	Source       string          `json:"source"`
	Uploader     *agent.Identity `json:"uploader,omitempty"`
	SolanaWallet string          `json:"solana_wallet,omitempty"` // artist tip address
	OnChain      bool            `json:"on_chain"`
	SolSig       string          `json:"sol_sig,omitempty"` // Solana tx sig
	TipTotal     uint64          `json:"tip_total"`
	PlayCount    uint64          `json:"play_count"`
	AddedBy      *agent.Identity `json:"added_by,omitempty"`
	AddedAt      time.Time       `json:"added_at"`
}

// StationEvent is emitted for terminal UI updates.
type StationEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	Time    time.Time   `json:"time"`
}

// Event type constants.
const (
	EventTrackStart    = "track_start"
	EventTrackEnd      = "track_end"
	EventDJJoined      = "dj_joined"
	EventDJLeft        = "dj_left"
	EventListenerJoin  = "listener_join"
	EventListenerLeft  = "listener_left"
	EventTipReceived   = "tip_received"
	EventNewBlock      = "new_block"
	EventChatMessage   = "chat_message"
	EventBalanceUpdate = "balance_update"
	EventSolanaOp      = "solana_op"
)

// New creates a new CLAWD FM station node.
func New(name string) (*Station, error) {
	id, priv, err := agent.NewIdentity(name, agent.RoleDJ)
	if err != nil {
		return nil, fmt.Errorf("station: %w", err)
	}

	ledger := blockchain.NewLedger(priv.Public().(ed25519.PublicKey))
	network := p2p.NewNetwork(id, priv, ledger)

	ctx, cancel := context.WithCancel(context.Background())

	s := &Station{
		Name:      name,
		Identity:  id,
		PrivKey:   priv,
		Ledger:    ledger,
		Network:   network,
		Agents:    make(map[string]*agent.Agent),
		Queue:     make([]*Track, 0),
		streamOut: make(chan *p2p.StreamChunk, 1024),
		events:    make(chan StationEvent, 512),
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	network.AgentReg.Register(id)

	network.SetCallbacks(
		s.onStreamChunk,
		s.onBlockReceived,
		s.onAgentJoin,
		s.onAgentLeave,
		s.onChatMessage,
	)

	return s, nil
}

// WithSolana attaches a Solana client to the station.
func (s *Station) WithSolana(client *solanaclient.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Solana = client
}

// Start boots the station and begins broadcasting.
func (s *Station) Start(addr string) error {
	if err := s.Network.Start(addr); err != nil {
		return fmt.Errorf("station: network: %w", err)
	}
	go s.runLoop()
	return nil
}

// Stop gracefully shuts down the station.
func (s *Station) Stop() error {
	s.cancel()
	<-s.done
	for _, a := range s.Agents {
		_ = a.Stop()
	}
	return s.Network.Stop()
}

// AddAgent registers and starts an agent on this station.
func (s *Station) AddAgent(a *agent.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Agents[a.Identity.ID] = a
	if err := a.Start(); err != nil {
		return err
	}
	a.SetStatus(agent.StatusListening)
	return nil
}

// RemoveAgent stops and removes an agent.
func (s *Station) RemoveAgent(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.Agents[id]
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}
	_ = a.Stop()
	delete(s.Agents, id)
	return nil
}

// CreateDJAgent creates a DJ agent for the station.
func (s *Station) CreateDJAgent(name string) (*agent.Agent, error) {
	id, priv, err := agent.NewIdentity(name, agent.RoleDJ)
	if err != nil {
		return nil, err
	}
	a := agent.New(id, priv)
	a.SetHandlers(
		nil, nil,
		func(ctx context.Context) error {
			s.mu.Lock()
			s.CurrentDJ = id
			s.mu.Unlock()
			s.emit(StationEvent{Type: EventDJJoined, Payload: id, Time: time.Now()})
			return nil
		},
		func(ctx context.Context) error {
			s.mu.Lock()
			s.CurrentDJ = nil
			s.Streaming = false
			s.mu.Unlock()
			s.emit(StationEvent{Type: EventDJLeft, Payload: id, Time: time.Now()})
			return nil
		},
	)
	a.Subscribe("track_request", "chat", "tip")
	return a, nil
}

// CreateListenerAgent creates a listener agent.
func (s *Station) CreateListenerAgent(name string) (*agent.Agent, error) {
	id, priv, err := agent.NewIdentity(name, agent.RoleListener)
	if err != nil {
		return nil, err
	}
	a := agent.New(id, priv)
	a.SetHandlers(
		nil, nil,
		func(ctx context.Context) error {
			s.mu.Lock()
			s.Listeners++
			count := s.Listeners
			s.mu.Unlock()
			fmt.Printf("[clawd-fm] %s tuned in (%d listeners)\n", name, count)
			s.emit(StationEvent{Type: EventListenerJoin, Payload: id, Time: time.Now()})
			return nil
		},
		func(ctx context.Context) error {
			s.mu.Lock()
			if s.Listeners > 0 {
				s.Listeners--
			}
			count := s.Listeners
			s.mu.Unlock()
			fmt.Printf("[clawd-fm] %s left (%d listeners)\n", name, count)
			s.emit(StationEvent{Type: EventListenerLeft, Payload: id, Time: time.Now()})
			return nil
		},
	)
	a.Subscribe("stream", "chat")
	return a, nil
}

// QueueTrack adds a track to the station queue.
func (s *Station) QueueTrack(t *Track) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.AddedAt = time.Now()
	s.Queue = append(s.Queue, t)
}

// GetQueue returns a snapshot of the current track queue.
func (s *Station) GetQueue() []*Track {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Track, len(s.Queue))
	copy(out, s.Queue)
	return out
}

// QueueTrackFromAPI creates a track from API input, registers it on-chain, and queues it.
func (s *Station) QueueTrackFromAPI(title, artist, source, cid string) *Track {
	if artist == "" {
		artist = "unknown"
	}
	if source == "" {
		source = "api"
	}
	if cid == "" {
		h := sha256.Sum256([]byte(title + artist + time.Now().String()))
		cid = hex.EncodeToString(h[:])
	}
	t := &Track{
		CID:     cid,
		Title:   title,
		Artist:  artist,
		Source:  source,
		AddedBy: s.Identity,
		AddedAt: time.Now(),
	}
	if err := s.RegisterTrackOnChain(t); err != nil {
		fmt.Printf("[api] warn: register track: %v\n", err)
	}
	s.QueueTrack(t)
	return t
}

// RegisterTrackOnChain records track metadata on Solana (via Memo program)
// and writes to the local ledger. The Solana tx signature is stored in SolSig.
func (s *Station) RegisterTrackOnChain(t *Track) error {
	// Write to local ledger first.
	acct := s.Ledger.GetBalance(s.PrivKey.Public().(ed25519.PublicKey))
	op, err := blockchain.NewOperation(
		blockchain.OpRegisterTrack,
		s.PrivKey,
		nil,
		0,
		acct,
		blockchain.TrackRecord{CID: t.CID, Title: t.Title, Artist: t.Artist},
	)
	if err != nil {
		return fmt.Errorf("register track (local): %w", err)
	}
	last := s.Ledger.LastBlock()
	block := &blockchain.Block{
		Index:     last.Index + 1,
		PrevHash:  last.Hash,
		Timestamp: time.Now(),
		Ops:       []blockchain.Operation{op},
	}
	if err := s.Ledger.AddBlock(block); err != nil {
		return fmt.Errorf("add block: %w", err)
	}
	s.Network.BroadcastBlock(block)

	// Write to Solana if client is attached.
	s.mu.RLock()
	sc := s.Solana
	s.mu.RUnlock()

	if sc != nil {
		sig, err := sc.RegisterTrack(s.ctx, t.CID, t.Title, t.Artist)
		if err != nil {
			fmt.Printf("[solana] warn: track memo failed: %v\n", err)
		} else {
			s.mu.Lock()
			t.SolSig = sig
			s.mu.Unlock()
			s.emit(StationEvent{
				Type:    EventSolanaOp,
				Payload: map[string]string{"op": "register_track", "sig": sig, "cid": t.CID},
				Time:    time.Now(),
			})
		}
	}

	t.OnChain = true
	return nil
}

// TipTrack sends a SOL tip to a track's artist wallet on Solana.
// Falls back to local ledger tip when Solana is not configured.
func (s *Station) TipTrack(cid string, lamports uint64) error {
	s.mu.RLock()
	sc := s.Solana
	s.mu.RUnlock()

	track := s.Ledger.GetTrack(cid)

	if sc != nil && track != nil && len(track.Uploader) == 32 {
		dest := solana.PublicKeyFromBytes(track.Uploader)
		sig, err := sc.TipArtist(s.ctx, dest, lamports)
		if err != nil {
			return fmt.Errorf("sol tip: %w", err)
		}
		s.emit(StationEvent{
			Type:    EventTipReceived,
			Payload: map[string]interface{}{"cid": cid, "lamports": lamports, "sig": sig},
			Time:    time.Now(),
		})
		return nil
	}

	// Local ledger fallback.
	if track == nil {
		return fmt.Errorf("track %s not found on chain", cid)
	}
	acct := s.Ledger.GetBalance(s.PrivKey.Public().(ed25519.PublicKey))
	op, err := blockchain.NewOperation(
		blockchain.OpTipTrack,
		s.PrivKey,
		ed25519.PublicKey(track.Uploader),
		lamports,
		acct,
		struct{ CID string }{CID: cid},
	)
	if err != nil {
		return err
	}
	last := s.Ledger.LastBlock()
	block := &blockchain.Block{
		Index:     last.Index + 1,
		PrevHash:  last.Hash,
		Timestamp: time.Now(),
		Ops:       []blockchain.Operation{op},
	}
	if err := s.Ledger.AddBlock(block); err != nil {
		return err
	}
	s.emit(StationEvent{
		Type:    EventTipReceived,
		Payload: map[string]interface{}{"cid": cid, "lamports": lamports},
		Time:    time.Now(),
	})
	s.Network.BroadcastBlock(block)
	return nil
}

// CreatePlaylistOnChain registers a playlist on Solana and the local ledger.
func (s *Station) CreatePlaylistOnChain(name string, cids []string) error {
	acct := s.Ledger.GetBalance(s.PrivKey.Public().(ed25519.PublicKey))
	op, err := blockchain.NewOperation(
		blockchain.OpCreatePlaylist,
		s.PrivKey,
		nil,
		0,
		acct,
		blockchain.PlaylistRecord{Name: name, TrackCIDs: cids},
	)
	if err != nil {
		return err
	}
	last := s.Ledger.LastBlock()
	block := &blockchain.Block{
		Index:     last.Index + 1,
		PrevHash:  last.Hash,
		Timestamp: time.Now(),
		Ops:       []blockchain.Operation{op},
	}
	if err := s.Ledger.AddBlock(block); err != nil {
		return err
	}
	s.Network.BroadcastBlock(block)

	s.mu.RLock()
	sc := s.Solana
	s.mu.RUnlock()

	if sc != nil {
		sig, err := sc.CreatePlaylist(s.ctx, name, cids)
		if err != nil {
			fmt.Printf("[solana] warn: playlist memo failed: %v\n", err)
		} else {
			s.emit(StationEvent{
				Type:    EventSolanaOp,
				Payload: map[string]string{"op": "create_playlist", "sig": sig, "name": name},
				Time:    time.Now(),
			})
		}
	}
	return nil
}

// SendChat broadcasts a chat message from an agent.
func (s *Station) SendChat(from *agent.Identity, text string) {
	s.Network.BroadcastChat(&p2p.ChatMessage{
		From:      from,
		Text:      text,
		Timestamp: time.Now(),
	})
}

// SolanaBalance returns the station wallet's SOL balance.
func (s *Station) SolanaBalance(ctx context.Context) (float64, error) {
	s.mu.RLock()
	sc := s.Solana
	s.mu.RUnlock()
	if sc == nil {
		return 0, fmt.Errorf("solana not configured")
	}
	return sc.BalanceSOL(ctx)
}

// SolanaPublicKey returns the station's Solana wallet address.
func (s *Station) SolanaPublicKey() string {
	s.mu.RLock()
	sc := s.Solana
	s.mu.RUnlock()
	if sc == nil {
		return "(no wallet)"
	}
	return sc.PublicKeyBase58()
}

// StreamChunkOut returns the channel for outbound audio chunks.
func (s *Station) StreamChunkOut() <-chan *p2p.StreamChunk {
	return s.streamOut
}

// Events returns the station event bus.
func (s *Station) Events() <-chan StationEvent {
	return s.events
}

// Balance returns the local ledger token balance.
func (s *Station) Balance() uint64 {
	return s.Ledger.GetBalance(s.PrivKey.Public().(ed25519.PublicKey))
}

// BlockHeight returns the current local chain height.
func (s *Station) BlockHeight() uint64 {
	return s.Ledger.Len()
}

// GetNowPlaying returns the currently playing track.
func (s *Station) GetNowPlaying() *Track {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.NowPlaying
}

// Stats returns a live snapshot of station metrics.
func (s *Station) Stats() StationStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StationStats{
		Name:        s.Name,
		Listeners:   s.Listeners,
		QueueLen:    len(s.Queue),
		BlockHeight: s.Ledger.Len(),
		Balance:     s.Ledger.GetBalance(s.PrivKey.Public().(ed25519.PublicKey)),
		Agents:      len(s.Agents),
		Peers:       len(s.Network.Peers),
		Streaming:   s.Streaming,
		NowPlaying:  s.NowPlaying,
		SolWallet:   s.solanaWallet(),
		HasSolana:   s.Solana != nil,
	}
}

func (s *Station) solanaWallet() string {
	if s.Solana == nil {
		return ""
	}
	return s.Solana.PublicKeyBase58()
}

// StationStats is a live snapshot of station health.
type StationStats struct {
	Name        string `json:"name"`
	Listeners   int    `json:"listeners"`
	QueueLen    int    `json:"queue_len"`
	BlockHeight uint64 `json:"block_height"`
	Balance     uint64 `json:"balance"`
	Agents      int    `json:"agents"`
	Peers       int    `json:"peers"`
	Streaming   bool   `json:"streaming"`
	NowPlaying  *Track `json:"now_playing,omitempty"`
	SolWallet   string `json:"sol_wallet,omitempty"`
	HasSolana   bool   `json:"has_solana"`
}

// Network callbacks.

func (s *Station) onStreamChunk(chunk *p2p.StreamChunk) {
	s.mu.RLock()
	for _, a := range s.Agents {
		if a.Identity.Role == agent.RoleListener {
			a.Send(&agent.Message{
				From:      s.Identity,
				Type:      "stream",
				Payload:   chunk.Data,
				Timestamp: time.Now(),
			})
		}
	}
	s.mu.RUnlock()

	select {
	case s.streamOut <- chunk:
	default:
	}
}

func (s *Station) onBlockReceived(b *blockchain.Block) {
	if err := s.Ledger.AddBlock(b); err != nil {
		return
	}
	s.emit(StationEvent{Type: EventNewBlock, Payload: b.Index, Time: time.Now()})
}

func (s *Station) onAgentJoin(id *agent.Identity) {
	s.Network.AgentReg.Register(id)
	s.emit(StationEvent{Type: EventDJJoined, Payload: id, Time: time.Now()})
}

func (s *Station) onAgentLeave(id *agent.Identity) {
	s.emit(StationEvent{Type: EventDJLeft, Payload: id, Time: time.Now()})
}

func (s *Station) onChatMessage(msg *p2p.ChatMessage) {
	s.mu.RLock()
	for _, a := range s.Agents {
		a.Send(&agent.Message{
			From:      msg.From,
			Type:      "chat",
			Payload:   []byte(msg.Text),
			Timestamp: msg.Timestamp,
		})
	}
	s.mu.RUnlock()
	s.emit(StationEvent{Type: EventChatMessage, Payload: msg, Time: msg.Timestamp})
}

func (s *Station) runLoop() {
	defer close(s.done)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-tick.C:
			s.tick()
		}
	}
}

func (s *Station) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.CurrentDJ == nil && len(s.Queue) > 0 {
		t := s.Queue[0]
		s.Queue = s.Queue[1:]
		s.NowPlaying = t
		t.PlayCount++
		s.emit(StationEvent{Type: EventTrackStart, Payload: t, Time: time.Now()})
	}
}

func (s *Station) emit(ev StationEvent) {
	select {
	case s.events <- ev:
	default:
	}
}

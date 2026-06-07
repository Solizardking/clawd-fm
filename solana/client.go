// Package solanaclient wraps gagliardetto/solana-go for CLAWD FM on-chain operations.
// Track metadata is written via the Memo program (raw instruction — no extra package).
// Tips are real SOL transfers via the System program.
//
// Integrates: github.com/solana-foundation/solana-go (gagliardetto/solana-go)
package solanaclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
)

const (
	LamportsPerSOL = 1_000_000_000

	// MemoPrefix is prepended to all on-chain track/playlist data.
	MemoPrefix = "CLAWDFM:"

	// memoProgramID is the canonical Solana Memo v2 program address.
	memoProgramID = "MemoSq4gqABAXKb96qnH8TysNcWxMyWCqXgDLGmfcHr"
)

// Network selects the Solana cluster.
type Network string

const (
	Mainnet Network = "mainnet"
	Devnet  Network = "devnet"
	Testnet Network = "testnet"
	Local   Network = "localnet"
)

func (n Network) Endpoint() string {
	switch n {
	case Devnet:
		return rpc.DevNet_RPC
	case Testnet:
		return rpc.TestNet_RPC
	case Local:
		return "http://127.0.0.1:8899"
	default:
		return rpc.MainNet_RPC
	}
}

// Client is the CLAWD FM Solana client — one per running station.
type Client struct {
	rpc    *rpc.Client
	wallet solana.PrivateKey
	net    Network
}

// New creates a client connected to network. walletBase58 is the station's
// private key in base58; if empty, a fresh ephemeral key is generated.
func New(network Network, walletBase58 string) (*Client, error) {
	var priv solana.PrivateKey
	var err error

	if walletBase58 == "" {
		priv, err = solana.NewRandomPrivateKey()
		if err != nil {
			return nil, fmt.Errorf("solana: generate wallet: %w", err)
		}
	} else {
		priv, err = solana.PrivateKeyFromBase58(walletBase58)
		if err != nil {
			return nil, fmt.Errorf("solana: parse wallet: %w", err)
		}
	}

	return &Client{
		rpc:    rpc.New(network.Endpoint()),
		wallet: priv,
		net:    network,
	}, nil
}

// NewFromKeyfile loads a wallet from a Solana CLI JSON keypair file.
func NewFromKeyfile(network Network, path string) (*Client, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("solana: read keypair file: %w", err)
	}
	var raw []byte
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("solana: parse keypair JSON: %w", err)
	}
	return &Client{
		rpc:    rpc.New(network.Endpoint()),
		wallet: solana.PrivateKey(raw),
		net:    network,
	}, nil
}

// PublicKey returns the station wallet public key.
func (c *Client) PublicKey() solana.PublicKey { return c.wallet.PublicKey() }

// PublicKeyBase58 returns the wallet public key as a base58 string.
func (c *Client) PublicKeyBase58() string { return c.wallet.PublicKey().String() }

// PrivateKeyBase58 returns the private key in base58.
func (c *Client) PrivateKeyBase58() string { return c.wallet.String() }

// Network returns the connected cluster.
func (c *Client) Network() Network { return c.net }

// BalanceLamports returns the SOL balance in lamports for the station wallet.
func (c *Client) BalanceLamports(ctx context.Context) (uint64, error) {
	out, err := c.rpc.GetBalance(ctx, c.wallet.PublicKey(), rpc.CommitmentConfirmed)
	if err != nil {
		return 0, fmt.Errorf("solana: get balance: %w", err)
	}
	return out.Value, nil
}

// BalanceSOL returns the balance as a SOL float.
func (c *Client) BalanceSOL(ctx context.Context) (float64, error) {
	lamps, err := c.BalanceLamports(ctx)
	if err != nil {
		return 0, err
	}
	return float64(lamps) / LamportsPerSOL, nil
}

// RegisterTrack writes track metadata on-chain via the Memo program.
// Returns the Solana transaction signature string.
func (c *Client) RegisterTrack(ctx context.Context, cid, title, artist string) (string, error) {
	data := fmt.Sprintf(`%s{"cid":%q,"title":%q,"artist":%q,"station":"CLAWD FM"}`,
		MemoPrefix, cid, title, artist)
	sig, err := c.sendMemo(ctx, data)
	if err != nil {
		return "", fmt.Errorf("solana: register track: %w", err)
	}
	return sig.String(), nil
}

// CreatePlaylist writes a playlist manifest on-chain via the Memo program.
func (c *Client) CreatePlaylist(ctx context.Context, name string, cids []string) (string, error) {
	cidsJSON, _ := json.Marshal(cids)
	data := fmt.Sprintf(`%s{"type":"playlist","name":%q,"tracks":%s,"station":"CLAWD FM"}`,
		MemoPrefix, name, string(cidsJSON))
	sig, err := c.sendMemo(ctx, data)
	if err != nil {
		return "", fmt.Errorf("solana: create playlist: %w", err)
	}
	return sig.String(), nil
}

// TipArtist sends lamports from the station wallet to an artist's pubkey.
func (c *Client) TipArtist(ctx context.Context, to solana.PublicKey, lamports uint64) (string, error) {
	blockhash, err := c.rpc.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("solana: get blockhash: %w", err)
	}

	instr := system.NewTransferInstruction(lamports, c.wallet.PublicKey(), to).Build()

	tx, err := solana.NewTransaction(
		[]solana.Instruction{instr},
		blockhash.Value.Blockhash,
		solana.TransactionPayer(c.wallet.PublicKey()),
	)
	if err != nil {
		return "", fmt.Errorf("solana: build tip tx: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(c.wallet.PublicKey()) {
			return &c.wallet
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("solana: sign tip tx: %w", err)
	}

	sig, err := c.rpc.SendTransaction(ctx, tx)
	if err != nil {
		return "", fmt.Errorf("solana: send tip tx: %w", err)
	}
	return sig.String(), nil
}

// RequestAirdrop requests devnet/testnet SOL (will fail on mainnet).
func (c *Client) RequestAirdrop(ctx context.Context, lamports uint64) (string, error) {
	if c.net == Mainnet {
		return "", fmt.Errorf("solana: airdrop not available on mainnet")
	}
	sig, err := c.rpc.RequestAirdrop(ctx, c.wallet.PublicKey(), lamports, rpc.CommitmentConfirmed)
	if err != nil {
		return "", fmt.Errorf("solana: airdrop: %w", err)
	}
	return sig.String(), nil
}

// SaveKeypair writes the wallet to a JSON file (Solana CLI compatible format).
func (c *Client) SaveKeypair(path string) error {
	raw := []byte(c.wallet)
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// WalletInfo returns a printable wallet snapshot.
func (c *Client) WalletInfo(ctx context.Context) WalletSummary {
	bal, _ := c.BalanceLamports(ctx)
	return WalletSummary{
		PublicKey: c.wallet.PublicKey().String(),
		Network:   string(c.net),
		Lamports:  bal,
		SOL:       float64(bal) / LamportsPerSOL,
	}
}

// WalletSummary is a display-friendly wallet snapshot.
type WalletSummary struct {
	PublicKey string  `json:"public_key"`
	Network   string  `json:"network"`
	Lamports  uint64  `json:"lamports"`
	SOL       float64 `json:"sol"`
}

// sendMemo submits arbitrary bytes to the Memo v2 program.
// We build the instruction manually (no programs/memo package needed).
func (c *Client) sendMemo(ctx context.Context, data string) (solana.Signature, error) {
	blockhash, err := c.rpc.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("get blockhash: %w", err)
	}

	progID := solana.MustPublicKeyFromBase58(memoProgramID)
	instr := solana.NewInstruction(
		progID,
		solana.AccountMetaSlice{
			solana.NewAccountMeta(c.wallet.PublicKey(), false, true),
		},
		[]byte(data),
	)

	tx, err := solana.NewTransaction(
		[]solana.Instruction{instr},
		blockhash.Value.Blockhash,
		solana.TransactionPayer(c.wallet.PublicKey()),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("build memo tx: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(c.wallet.PublicKey()) {
			return &c.wallet
		}
		return nil
	})
	if err != nil {
		return solana.Signature{}, fmt.Errorf("sign memo tx: %w", err)
	}

	sig, err := c.rpc.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("send memo tx: %w", err)
	}
	return sig, nil
}

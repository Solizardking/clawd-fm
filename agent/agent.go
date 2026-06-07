// Package agent defines the autonomous agent system for clawdamp.
// Agents are autonomous entities that can stream music, curate playlists,
// trade on-chain, and interact with other agents over the p2p network.
package agent

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Role defines what an agent does on the network.
type Role string

const (
	RoleDJ       Role = "dj"       // Curates and streams music
	RoleListener Role = "listener" // Listens and tips
	RoleCurator  Role = "curator"  // Builds playlists on-chain
	RoleRelay    Role = "relay"    // Relays streams p2p
	RoleOracle   Role = "oracle"   // Feeds external data on-chain
)

// Status represents the current state of an agent.
type Status string

const (
	StatusIdle     Status = "idle"
	StatusStreaming Status = "streaming"
	StatusListening Status = "listening"
	StatusTrading  Status = "trading"
	StatusOffline  Status = "offline"
)

// Identity uniquely identifies an agent on the clawdamp network.
type Identity struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PublicKey []byte    `json:"public_key"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// NewIdentity creates a fresh agent identity with an Ed25519 keypair.
func NewIdentity(name string, role Role) (*Identity, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: generate key: %w", err)
	}
	return &Identity{
		ID:        uuid.New().String(),
		Name:      name,
		PublicKey: pub,
		Role:      role,
		CreatedAt: time.Now(),
	}, priv, nil
}

// Fingerprint returns a short hex identifier for logging.
func (id *Identity) Fingerprint() string {
	if len(id.PublicKey) < 4 {
		return hex.EncodeToString(id.PublicKey)
	}
	return hex.EncodeToString(id.PublicKey[:4])
}

// Agent is an autonomous entity on the clawdamp network.
type Agent struct {
	Identity   *Identity
	PrivateKey ed25519.PrivateKey
	Status     Status
	mu         sync.RWMutex

	// Handlers for the agent lifecycle.
	onTick    func(ctx context.Context) error
	onMessage func(ctx context.Context, from *Identity, payload []byte) error
	onStart   func(ctx context.Context) error
	onStop    func(ctx context.Context) error

	// Subscriptions to events the agent cares about.
	subscriptions []string

	// Inbound message channel.
	inbox chan *Message

	// Lifecycle.
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// Message represents a message between agents.
type Message struct {
	From      *Identity `json:"from"`
	To        string    `json:"to,omitempty"`
	Type      string    `json:"type"`
	Payload   []byte    `json:"payload"`
	Signature []byte    `json:"signature"`
	Timestamp  time.Time `json:"timestamp"`
}

// New creates a new agent.
func New(id *Identity, priv ed25519.PrivateKey) *Agent {
	ctx, cancel := context.WithCancel(context.Background())
	return &Agent{
		Identity:   id,
		PrivateKey: priv,
		Status:     StatusIdle,
		inbox:      make(chan *Message, 64),
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
}

// SetHandlers configures the agent's behavior callbacks.
func (a *Agent) SetHandlers(onTick, onMessage, onStart, onStop func(ctx context.Context) error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onTick = onTick
	a.onMessage = onMessage
	a.onStart = onStart
	a.onStop = onStop
}

// Subscribe registers interest in a message type.
func (a *Agent) Subscribe(types ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.subscriptions = append(a.subscriptions, types...)
}

// Send delivers a message to this agent's inbox.
func (a *Agent) Send(msg *Message) {
	select {
	case a.inbox <- msg:
	default:
		// Drop if inbox is full (backpressure).
	}
}

// SetStatus updates the agent's status.
func (a *Agent) SetStatus(s Status) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Status = s
}

// GetStatus returns the current status.
func (a *Agent) GetStatus() Status {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Status
}

// Start begins the agent's event loop.
func (a *Agent) Start() error {
	a.mu.RLock()
	onStart := a.onStart
	a.mu.RUnlock()

	if onStart != nil {
		if err := onStart(a.ctx); err != nil {
			return fmt.Errorf("agent %s: start: %w", a.Identity.Name, err)
		}
	}

	go a.loop()
	a.SetStatus(StatusIdle)
	return nil
}

// Stop gracefully shuts down the agent.
func (a *Agent) Stop() error {
	a.cancel()
	<-a.done

	a.mu.RLock()
	onStop := a.onStop
	a.mu.RUnlock()

	if onStop != nil {
		return onStop(a.ctx)
	}
	return nil
}

func (a *Agent) loop() {
	defer close(a.done)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.mu.RLock()
			onTick := a.onTick
			a.mu.RUnlock()
			if onTick != nil {
				_ = onTick(a.ctx)
			}
		case msg := <-a.inbox:
			a.mu.RLock()
			onMessage := a.onMessage
			a.mu.RUnlock()
			if onMessage != nil {
				_ = onMessage(a.ctx, msg.From, msg.Payload)
			}
		}
	}
}

// SignMessage signs a payload with the agent's private key.
func (a *Agent) SignMessage(payload []byte) []byte {
	sig := ed25519.Sign(a.PrivateKey, payload)
	return sig
}

// VerifyMessage verifies a message signature from another agent.
func (a *Agent) VerifyMessage(msg *Message) bool {
	return ed25519.Verify(msg.From.PublicKey, msg.Payload, msg.Signature)
}
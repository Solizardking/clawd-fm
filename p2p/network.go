// Package p2p implements the peer-to-peer networking layer for clawdamp.
// Agents discover each other, relay messages, and stream audio chunks
// over a gossip protocol. It uses libp2p-style peer IDs and topics.
package p2p

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"clawdamp/agent"
	"clawdamp/blockchain"
)

// Topic names for the clawdamp gossip network.
const (
	TopicAgents      = "clawdamp/agents"
	TopicBlocks      = "clawdamp/blocks"
	TopicStreams     = "clawdamp/streams"
	TopicChat        = "clawdamp/chat"
	TopicMarket      = "clawdamp/market"
)

// Peer represents a remote node on the clawdamp network.
type Peer struct {
	ID         string
	Addr       net.Addr
	Identity   *agent.Identity
	LastSeen   time.Time
	Latency    time.Duration
	Streaming  bool
}

// StreamChunk is an audio chunk relayed over p2p.
type StreamChunk struct {
	StreamID  string    `json:"stream_id"`
	Seq       uint64    `json:"seq"`
	Data      []byte    `json:"data"`
	Codec     string    `json:"codec"`
	Bitrate   int       `json:"bitrate"`
	Timestamp time.Time `json:"timestamp"`
	DJ        []byte    `json:"dj"` // Public key of the streaming agent
}

// Network manages the peer-to-peer overlay for clawdamp.
type Network struct {
	mu       sync.RWMutex
	Self     *agent.Identity
	PrivKey  ed25519.PrivateKey
	Peers    map[string]*Peer          // peer ID -> peer
	Topics   map[string][]string       // topic -> subscribed peer IDs
	Ledger   *blockchain.Ledger
	AgentReg *AgentRegistry

	// Inbound channels.
	streamCh chan *StreamChunk
	blockCh  chan *blockchain.Block
	agentCh  chan *agent.Identity
	chatCh   chan *ChatMessage

	// Network layer.
	listener  net.Listener
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}

	// Callbacks.
	onStream     func(*StreamChunk)
	onBlock      func(*blockchain.Block)
	onAgentJoin  func(*agent.Identity)
	onAgentLeave func(*agent.Identity)
	onChat       func(*ChatMessage)
}

// ChatMessage is a text message between agents.
type ChatMessage struct {
	From      *agent.Identity `json:"from"`
	To        string          `json:"to,omitempty"`
	Text      string          `json:"text"`
	Timestamp time.Time       `json:"timestamp"`
}

// AgentRegistry maintains a directory of known agents.
type AgentRegistry struct {
	mu      sync.RWMutex
	Agents  map[string]*agent.Identity // ID -> identity
}

// NewAgentRegistry creates a fresh agent registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		Agents: make(map[string]*agent.Identity),
	}
}

// Register adds an agent to the registry.
func (ar *AgentRegistry) Register(id *agent.Identity) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	ar.Agents[id.ID] = id
}

// FindByRole returns all agents matching a role.
func (ar *AgentRegistry) FindByRole(role agent.Role) []*agent.Identity {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	var out []*agent.Identity
	for _, a := range ar.Agents {
		if a.Role == role {
			out = append(out, a)
		}
	}
	return out
}

// All returns all registered agents.
func (ar *AgentRegistry) All() []*agent.Identity {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	out := make([]*agent.Identity, 0, len(ar.Agents))
	for _, a := range ar.Agents {
		out = append(out, a)
	}
	return out
}

// Remove removes an agent from the registry.
func (ar *AgentRegistry) Remove(id string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	delete(ar.Agents, id)
}

// NewNetwork creates a new p2p network node.
func NewNetwork(self *agent.Identity, priv ed25519.PrivateKey, ledger *blockchain.Ledger) *Network {
	ctx, cancel := context.WithCancel(context.Background())
	return &Network{
		Self:     self,
		PrivKey:  priv,
		Peers:    make(map[string]*Peer),
		Topics: map[string][]string{
			TopicAgents:  {},
			TopicBlocks:  {},
			TopicStreams: {},
			TopicChat:    {},
			TopicMarket:  {},
		},
		Ledger:   ledger,
		AgentReg: NewAgentRegistry(),
		streamCh: make(chan *StreamChunk, 256),
		blockCh:  make(chan *blockchain.Block, 64),
		agentCh:  make(chan *agent.Identity, 64),
		chatCh:   make(chan *ChatMessage, 128),
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
}

// SetCallbacks configures event handlers.
func (n *Network) SetCallbacks(onStream func(*StreamChunk), onBlock func(*blockchain.Block), onAgentJoin, onAgentLeave func(*agent.Identity), onChat func(*ChatMessage)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onStream = onStream
	n.onBlock = onBlock
	n.onAgentJoin = onAgentJoin
	n.onAgentLeave = onAgentLeave
	n.onChat = onChat
}

// AddPeer registers a new peer.
func (n *Network) AddPeer(p *Peer) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Peers[p.ID] = p
	if p.Identity != nil {
		n.AgentReg.Register(p.Identity)
		if n.onAgentJoin != nil {
			n.onAgentJoin(p.Identity)
		}
	}
}

// RemovePeer unregisters a peer.
func (n *Network) RemovePeer(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if p, ok := n.Peers[id]; ok {
		if p.Identity != nil && n.onAgentLeave != nil {
			n.onAgentLeave(p.Identity)
		}
	}
	delete(n.Peers, id)
}

// SubscribeToTopic joins a gossip topic.
func (n *Network) SubscribeToTopic(peerID, topic string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, t := range n.Topics[topic] {
		if t == peerID {
			return
		}
	}
	n.Topics[topic] = append(n.Topics[topic], peerID)
}

// BroadcastStream sends an audio chunk to all stream subscribers.
func (n *Network) BroadcastStream(chunk *StreamChunk) {
	select {
	case n.streamCh <- chunk:
		n.mu.RLock()
		onStream := n.onStream
		n.mu.RUnlock()
		if onStream != nil {
			onStream(chunk)
		}
	default:
	}
}

// BroadcastBlock announces a new block to the network.
func (n *Network) BroadcastBlock(b *blockchain.Block) {
	select {
	case n.blockCh <- b:
		n.mu.RLock()
		onBlock := n.onBlock
		n.mu.RUnlock()
		if onBlock != nil {
			onBlock(b)
		}
	default:
	}
}

// BroadcastChat sends a chat message to the network.
func (n *Network) BroadcastChat(msg *ChatMessage) {
	select {
	case n.chatCh <- msg:
		n.mu.RLock()
		onChat := n.onChat
		n.mu.RUnlock()
		if onChat != nil {
			onChat(msg)
		}
	default:
	}
}

// Announce broadcasts this agent's presence.
func (n *Network) Announce() {
	select {
	case n.agentCh <- n.Self:
	default:
	}
}

// Start begins the network event loop.
func (n *Network) Start(addr string) error {
	var err error
	n.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("p2p: listen %s: %w", addr, err)
	}

	fmt.Printf("[p2p] %s listening on %s\n", n.Self.Name, addr)

	go n.gossipLoop()
	return nil
}

// Stop shuts down the network.
func (n *Network) Stop() error {
	n.cancel()
	<-n.done
	if n.listener != nil {
		n.listener.Close()
	}
	return nil
}

func (n *Network) gossipLoop() {
	defer close(n.done)

	gossipTicker := time.NewTicker(5 * time.Second)
	defer gossipTicker.Stop()

	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-gossipTicker.C:
			n.gossip()
		case <-heartbeatTicker.C:
			n.Announce()
		}
	}
}

func (n *Network) gossip() {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Prune stale peers.
	now := time.Now()
	for id, p := range n.Peers {
		if now.Sub(p.LastSeen) > 2*time.Minute {
			go n.RemovePeer(id)
		}
	}
}

// ConnectPeer dials a remote peer.
func (n *Network) ConnectPeer(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("p2p: dial %s: %w", addr, err)
	}

	go n.handlePeerConn(conn, addr)
	return nil
}

func (n *Network) handlePeerConn(conn net.Conn, addr string) {
	defer conn.Close()

	dec := json.NewDecoder(conn)
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if err == io.EOF {
				return
			}
			continue
		}

		// Try to decode as various message types.
		var chunk StreamChunk
		if json.Unmarshal(raw, &chunk) == nil && chunk.StreamID != "" {
			n.BroadcastStream(&chunk)
			continue
		}

		var block blockchain.Block
		if json.Unmarshal(raw, &block) == nil && block.Index > 0 {
			n.BroadcastBlock(&block)
			continue
		}

		var chat ChatMessage
		if json.Unmarshal(raw, &chat) == nil && chat.Text != "" {
			n.BroadcastChat(&chat)
			continue
		}
	}
}

// PeersByRole returns peers matching a role.
func (n *Network) PeersByRole(role agent.Role) []*agent.Identity {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.AgentReg.FindByRole(role)
}

// OnlineDjs returns streaming DJ agents.
func (n *Network) OnlineDjs() []*agent.Identity {
	return n.PeersByRole(agent.RoleDJ)
}
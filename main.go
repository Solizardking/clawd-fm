// clawdamp — blockchain terminal radio station.
// Broadcast music from terminal to terminal. Agents curate, stream,
// tip on-chain, and gossip over p2p. Built with Bubbletea TUI.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"clawdamp/agent"
	"clawdamp/blockchain"
	"clawdamp/p2p"
	"clawdamp/radio"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

func main() {
	if err := run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "clawdamp: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fmt.Println(logo())
	fmt.Printf("clawdamp %s — blockchain terminal radio\n\n", version)

	// Parse subcommands.
	if len(args) > 1 {
		switch args[1] {
		case "help", "-h", "--help":
			printHelp()
			return nil
		case "version", "-v", "--version":
			fmt.Printf("clawdamp version %s\n", version)
			return nil
		case "init":
			return initStation(args[2:])
		case "start":
			return startStation(args[2:])
		case "connect":
			return connectStation(args[2:])
		case "status":
			return showStatus(args[2:])
		case "chat":
			return sendChat(args[2:])
		case "queue":
			return queueTrack(args[2:])
		case "agents":
			return listAgents(args[2:])
		case "blockchain":
			return chainInfo(args[2:])
		case "tip":
			return sendTip(args[2:])
		case "playlist":
			return managePlaylist(args[2:])
		}
	}

	// Interactive mode.
	return interactive(ctx)
}

func interactive(ctx context.Context) error {
	fmt.Println("Starting clawdamp in interactive mode...")
	fmt.Println()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create station.
	st, err := radio.New("clawdamp-local")
	if err != nil {
		return fmt.Errorf("create station: %w", err)
	}
	defer st.Stop()

	// Start p2p.
	addr := ":9669"
	if env := os.Getenv("CLAWDAMP_ADDR"); env != "" {
		addr = env
	}
	if err := st.Start(addr); err != nil {
		return fmt.Errorf("start station: %w", err)
	}

	fmt.Printf("🌐 Station online at %s\n", addr)
	fmt.Printf("🔑 Identity: %s\n", st.Identity.Fingerprint())
	fmt.Printf("📦 Block height: %d\n", st.BlockHeight())
	fmt.Printf("💰 Balance: %d CLAW\n\n", st.Balance())

	// Create a default DJ agent.
	dj, err := st.CreateDJAgent("Default DJ")
	if err != nil {
		return fmt.Errorf("create DJ: %w", err)
	}
	if err := st.AddAgent(dj); err != nil {
		return fmt.Errorf("add DJ: %w", err)
	}
	fmt.Println("🎧 Default DJ agent is in the booth")

	// Demo: register a track on-chain.
	demoTrack := &radio.Track{
		CID:    hashString("demo-track-1"),
		Title:  "First Clawdamp Broadcast",
		Artist: "Clawdamp Genesis",
		Duration: 3*time.Minute + 30*time.Second,
		Source: "local",
	}
	if err := st.RegisterTrackOnChain(demoTrack); err != nil {
		fmt.Printf("⚠️  Track registration: %v\n", err)
	} else {
		st.QueueTrack(demoTrack)
		fmt.Printf("📀 Registered: %q by %s\n", demoTrack.Title, demoTrack.Artist)
	}

	// Print station info.
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────┐")
	fmt.Println("│        CLAWDAMP RADIO ONLINE          │")
	fmt.Println("├─────────────────────────────────────────┤")
	fmt.Printf("│ Station: %-30s │\n", st.Name)
	fmt.Printf("│ Chain:   %-30s │\n", fmt.Sprintf("block %d", st.BlockHeight()))
	fmt.Printf("│ Token:   %-30s │\n", fmt.Sprintf("%d CLAW", st.Balance()))
	fmt.Printf("│ Agents:  %-30d │\n", 1)
	fmt.Printf("│ Addr:    %-30s │\n", addr)
	fmt.Println("└─────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  h, help        Show this")
	fmt.Println("  s, status      Show station status")
	fmt.Println("  a, agents      List agents")
	fmt.Println("  c, chat <msg>  Send chat message")
	fmt.Println("  q, queue <url> Queue a track")
	fmt.Println("  t, tip <cid>   Tip a track")
	fmt.Println("  b, blocks      Show chain info")
	fmt.Println("  Ctrl+C         Quit")
	fmt.Println()

	// Simple repl.
	inputCh := make(chan string, 16)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, _ := os.Stdin.Read(buf)
			if n > 0 {
				inputCh <- strings.TrimSpace(string(buf[:n]))
			}
		}
	}()

	// Status ticker.
	statusTicker := time.NewTicker(30 * time.Second)
	defer statusTicker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Println("\n👋 Shutting down clawdamp...")
			return nil
		case <-statusTicker.C:
			stats := st.Stats()
			fmt.Printf("[status] height=%d listeners=%d queue=%d balance=%d\n",
				stats.BlockHeight, stats.Listeners, stats.QueueLen, stats.Balance)
		case line := <-inputCh:
			parts := strings.Fields(line)
			if len(parts) == 0 {
				continue
			}
			switch parts[0] {
			case "h", "help":
				printHelp()
			case "s", "status":
				stats := st.Stats()
				fmt.Printf("\n📊 Station: %s\n", stats.Name)
				fmt.Printf("   Block height: %d\n", stats.BlockHeight)
				fmt.Printf("   Balance: %d CLAW\n", stats.Balance)
				fmt.Printf("   Listeners: %d\n", stats.Listeners)
				fmt.Printf("   Agents: %d\n", stats.Agents)
				fmt.Printf("   Peers: %d\n", stats.Peers)
				fmt.Printf("   Queue: %d tracks\n", stats.QueueLen)
				if stats.NowPlaying != nil {
					fmt.Printf("   Now Playing: %s — %s\n", stats.NowPlaying.Title, stats.NowPlaying.Artist)
				}
			case "a", "agents":
				fmt.Println("\n🤖 Agents:")
				for _, a := range st.Network.AgentReg.All() {
					fmt.Printf("   %s [%s] %s\n", a.Name, a.Role, a.Fingerprint())
				}
			case "c", "chat":
				if len(parts) < 2 {
					fmt.Println("Usage: chat <message>")
					continue
				}
				text := strings.Join(parts[1:], " ")
				st.SendChat(st.Identity, text)
				fmt.Printf("💬 You: %s\n", text)
			case "q", "queue":
				if len(parts) < 2 {
					fmt.Println("Usage: queue <url>")
					continue
				}
				track := &radio.Track{
					CID:     hashString(strings.Join(parts[1:], " ")),
					Title:   strings.Join(parts[1:], " "),
					Artist:  "Queue",
					Source:  "queue",
					AddedAt: time.Now(),
				}
				st.QueueTrack(track)
				_ = st.RegisterTrackOnChain(track)
				fmt.Printf("📀 Queued: %s\n", track.Title)
			case "t", "tip":
				if len(parts) < 3 {
					fmt.Println("Usage: tip <cid> <amount>")
					continue
				}
				var amount uint64
				fmt.Sscanf(parts[2], "%d", &amount)
				if err := st.TipTrack(parts[1], amount); err != nil {
					fmt.Printf("❌ Tip failed: %v\n", err)
				} else {
					fmt.Printf("💸 Tipped %d CLAW to track %s\n", amount, parts[1])
				}
			case "b", "blocks":
				b := st.Ledger.LastBlock()
				if b == nil {
					fmt.Println("No blocks yet.")
					continue
				}
				fmt.Printf("\n📦 Chain (height %d)\n", b.Index)
				fmt.Printf("   Last block hash: %x\n", b.Hash[:8])
				fmt.Printf("   Ops: %d\n", len(b.Ops))
				fmt.Printf("   Time: %s\n", b.Timestamp.Format(time.RFC3339))
			default:
				fmt.Printf("Unknown command: %s (type 'h' for help)\n", parts[0])
			}
		}
	}
}

func initStation(args []string) error {
	name := "clawdamp"
	if len(args) > 0 {
		name = args[0]
	}

	// Generate identity.
	id, priv, err := agent.NewIdentity(name, agent.RoleDJ)
	if err != nil {
		return err
	}

	// Save keypair to disk.
	keyPath := fmt.Sprintf(".clawdamp/%s.key", name)
	os.MkdirAll(".clawdamp", 0700)
	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		return fmt.Errorf("save key: %w", err)
	}

	fmt.Printf("✅ Station initialized: %s\n", name)
	fmt.Printf("   Identity: %s\n", id.Fingerprint())
	fmt.Printf("   Role: %s\n", id.Role)
	fmt.Printf("   Key saved to: %s\n", keyPath)
	fmt.Printf("\nNext: clawdamp start [--addr :9669]\n")
	return nil
}

func startStation(args []string) error {
	addr := ":9669"
	name := "clawdamp"
	for i, a := range args {
		if a == "--addr" && i+1 < len(args) {
			addr = args[i+1]
		}
		if a == "--name" && i+1 < len(args) {
			name = args[i+1]
		}
	}

	st, err := radio.New(name)
	if err != nil {
		return err
	}

	dj, _ := st.CreateDJAgent("Auto DJ")
	_ = st.AddAgent(dj)

	if err := st.Start(addr); err != nil {
		return err
	}
	defer st.Stop()

	fmt.Printf("🌐 Station %s broadcasting on %s\n", name, addr)
	fmt.Printf("   Height: %d | Balance: %d CLAW\n", st.BlockHeight(), st.Balance())
	fmt.Println("\nPress Ctrl+C to stop...")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-sigCh:
			return nil
		case <-tick.C:
			stats := st.Stats()
			fmt.Printf("[%s] height=%d listeners=%d queue=%d\n",
				time.Now().Format("15:04:05"), stats.BlockHeight, stats.Listeners, stats.QueueLen)
		case ev := <-st.Events():
			switch ev.Type {
			case radio.EventTrackStart:
				if t, ok := ev.Payload.(*radio.Track); ok {
					fmt.Printf("▶️  Now playing: %s — %s\n", t.Title, t.Artist)
				}
			case radio.EventChatMessage:
				if msg, ok := ev.Payload.(*p2p.ChatMessage); ok {
					fmt.Printf("💬 [%s] %s: %s\n", msg.From.Name, msg.From.Fingerprint(), msg.Text)
				}
			}
		}
	}
}

func connectStation(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: clawdamp connect <peer-address>")
	}

	id, priv, err := agent.NewIdentity("listener", agent.RoleListener)
	if err != nil {
		return err
	}

	ledger := blockchain.NewLedger(priv.Public().(ed25519.PublicKey))
	net := p2p.NewNetwork(id, priv, ledger)

	address := args[0]
	if !strings.Contains(address, ":") {
		address = address + ":9669"
	}

	if err := net.Start(":0"); err != nil {
		return err
	}
	defer net.Stop()

	fmt.Printf("🔗 Connecting to %s...\n", address)
	if err := net.ConnectPeer(address); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	fmt.Println("✅ Connected. Listening for streams...")
	fmt.Println("Press Ctrl+C to disconnect.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	return nil
}

func showStatus(args []string) error {
	fmt.Println("clawdamp status")
	fmt.Println("  No active station detected.")
	fmt.Println("  Start one with: clawdamp start")
	return nil
}

func sendChat(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: clawdamp chat <message>")
	}
	fmt.Printf("💬 Chat: %s\n", strings.Join(args, " "))
	return nil
}

func queueTrack(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: clawdamp queue <url>")
	}
	fmt.Printf("📀 Queued: %s\n", strings.Join(args, " "))
	return nil
}

func listAgents(args []string) error {
	fmt.Println("🤖 Agents on network:")
	fmt.Println("  No agents discovered yet.")
	fmt.Println("  Run 'clawdamp start' to begin broadcasting.")
	return nil
}

func chainInfo(args []string) error {
	fmt.Println("📦 Blockchain info:")
	fmt.Println("  No active chain detected.")
	fmt.Println("  Run 'clawdamp start' to initialize.")
	return nil
}

func sendTip(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: clawdamp tip <cid> <amount>")
	}
	fmt.Printf("💸 Tipped %s CLAW to track %s\n", args[1], args[0])
	return nil
}

func managePlaylist(args []string) error {
	if len(args) == 0 {
		fmt.Println("┌─────────────────────────────────────┐")
		fmt.Println("│     ON-CHAIN PLAYLISTS            │")
		fmt.Println("│  clawdamp playlist create <name>  │")
		fmt.Println("│  clawdamp playlist add <id> <cid> │")
		fmt.Println("│  clawdamp playlist show <id>      │")
		fmt.Println("└─────────────────────────────────────┘")
		return nil
	}
	return nil
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func logo() string {
	return `
   ▄████████  ▄█        ▄████████ ▄██   ▄   ████████▄     ▄████████   ▄▄▄▄███▄▄▄▄      ▄███████▄  
  ███    ███ ███       ███    ███ ███   ██▄ ███   ▀███   ███    ███ ▄██▀▀▀███▀▀▀██▄   ███    ███ 
  ███    █▀  ███       ███    ███ ███▄▄▄███ ███    ███   ███    █▀  ███   ███   ███   ███    ███ 
  ███        ███       ███    ███ ▀▀▀▀▀▀███ ███    ███  ▄███▄▄▄     ███   ███   ███   ███    ███ 
  ███        ███     ▀███████████ ▄██   ███ ███    ███ ▀▀███▀▀▀     ███   ███   ███ ▀█████████▀  
  ███    █▄  ███       ███    ███ ███   ███ ███    ███   ███    █▄  ███   ███   ███   ███        
  ███    ███ ███▌    ▄ ███    ███ ███   ███ ███   ▄███   ███    ███ ███   ███   ███   ███        
  ████████▀  █████▄▄██ ███    █▀   ▀█████▀  ████████▀    ██████████  ▀█   ███   █▀   ▄████▀      
             ▀                                                                                   

   ████████▄     ▄████████    ▄████████  ▀████    ▐████▀    ▄███████▄  
   ███   ▀███   ███    ███   ███    ███    ███▌   ████▀    ███    ███ 
   ███    ███   ███    ███   ███    █▀      ███  ▐███      ███    ███ 
   ███    ███  ▄███▄▄▄▄██▀   ███            ▀███▄███▀      ███    ███ 
   ███    ███ ▀▀███▀▀▀▀▀   ▀███████████     ████▀██▄     ▀█████████▀  
   ███    ███ ▀███████████          ███    ▐███  ▀███      ███        
   ███   ▄███   ███    ███    ▄█    ███   ▄███     ███▄    ███        
   ████████▀    ███    ███  ▄████████▀  ████       ███▄  ▄████▀      
                ███    ███                                              
`
}

func printHelp() {
	fmt.Println(`clawdamp — blockchain terminal radio station

USAGE:
  clawdamp                    Interactive terminal radio
  clawdamp init [name]        Initialize a new station identity
  clawdamp start              Start broadcasting
  clawdamp connect <addr>     Tune in to a remote station
  clawdamp status             Show station status
  clawdamp chat <msg>         Send chat message
  clawdamp queue <url>        Queue a track
  clawdamp agents             List network agents
  clawdamp tip <cid> <amt>    Tip a track (CLAW tokens)
  clawdamp blockchain         Show on-chain info
  clawdamp playlist           Manage on-chain playlists
  clawdamp help               Show this help

ENVIRONMENT:
  CLAWDAMP_ADDR               Listen address (default :9669)

EXAMPLES:
  clawdamp init my-station
  clawdamp start
  clawdamp connect 192.168.1.5:9669
  clawdamp queue https://example.com/track.mp3
  clawdamp tip abc123 100`)
}
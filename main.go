// CLAWD FM — First Solana Onchain Terminal Radio Station
// Broadcast music from terminal to terminal. Agents curate, stream, and tip
// artists on Solana. Built for 24/7 deployment on any server.
//
// Integrates: github.com/solana-foundation/solana-go (gagliardetto/solana-go)
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

	clawdapi "clawdamp/api"
	"clawdamp/agent"
	"clawdamp/blockchain"
	"clawdamp/p2p"
	"clawdamp/radio"
	solanaclient "clawdamp/solana"
)

var version = "dev"

func main() {
	if err := run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "clawd-fm: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fmt.Println(logo())
	fmt.Printf("CLAWD FM %s — First Solana Onchain Terminal Radio\n\n", version)

	if len(args) > 1 {
		switch args[1] {
		case "help", "-h", "--help":
			printHelp()
			return nil
		case "version", "-v", "--version":
			fmt.Printf("CLAWD FM version %s\n", version)
			return nil
		case "init":
			return cmdInit(args[2:])
		case "start", "fm":
			return cmdStart(args[2:])
		case "connect":
			return cmdConnect(args[2:])
		case "status":
			return cmdStatus(args[2:])
		case "chat":
			return cmdChat(args[2:])
		case "queue":
			return cmdQueue(args[2:])
		case "agents":
			return cmdAgents(args[2:])
		case "blockchain", "chain":
			return cmdChain(args[2:])
		case "tip":
			return cmdTip(args[2:])
		case "playlist":
			return cmdPlaylist(args[2:])
		case "wallet":
			return cmdWallet(args[2:])
		case "airdrop":
			return cmdAirdrop(args[2:])
		case "daemon":
			return cmdDaemon(args[2:])
		}
	}

	return interactive(ctx)
}

// interactive is the full CLAWD FM terminal experience.
func interactive(ctx context.Context) error {
	fmt.Println("Starting CLAWD FM in interactive mode...")
	fmt.Println()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Build station.
	stationName := envOr("CLAWD_FM_NAME", "CLAWD FM")
	st, err := radio.New(stationName)
	if err != nil {
		return fmt.Errorf("create station: %w", err)
	}
	defer st.Stop()

	// Attach Solana if wallet is configured.
	solNet := solanaclient.Network(envOr("CLAWD_SOL_NETWORK", "devnet"))
	solWallet := envOr("CLAWD_SOL_WALLET", "")
	solKeyfile := envOr("CLAWD_SOL_KEYFILE", "")

	var sc *solanaclient.Client
	if solKeyfile != "" {
		sc, err = solanaclient.NewFromKeyfile(solNet, solKeyfile)
		if err != nil {
			fmt.Printf("[solana] warn: could not load keyfile %s: %v\n", solKeyfile, err)
		}
	} else {
		sc, err = solanaclient.New(solNet, solWallet)
		if err != nil {
			fmt.Printf("[solana] warn: wallet init failed: %v\n", err)
		}
	}
	if sc != nil {
		st.WithSolana(sc)
		fmt.Printf("[solana] wallet: %s\n", sc.PublicKeyBase58())
		fmt.Printf("[solana] network: %s\n", solNet)
	} else {
		fmt.Println("[solana] running in local-only mode (set CLAWD_SOL_WALLET or CLAWD_SOL_KEYFILE)")
	}
	fmt.Println()

	// Start p2p.
	addr := envOr("CLAWD_FM_ADDR", ":9669")
	if err := st.Start(addr); err != nil {
		return fmt.Errorf("start station: %w", err)
	}

	// Start HTTP API (SSE + REST for radio.x402.wtf frontend).
	httpAddr := envOr("CLAWD_HTTP_ADDR", ":8080")
	apiSrv := clawdapi.New(st)
	if err := apiSrv.Start(httpAddr); err != nil {
		return fmt.Errorf("start api: %w", err)
	}

	fmt.Printf("[clawd-fm] station: %s\n", stationName)
	fmt.Printf("[clawd-fm] p2p:     %s\n", addr)
	fmt.Printf("[clawd-fm] http:    %s\n", httpAddr)
	fmt.Printf("[clawd-fm] id:      %s\n", st.Identity.Fingerprint())
	fmt.Printf("[clawd-fm] height:  %d\n", st.BlockHeight())
	fmt.Println()

	// Create default DJ agent.
	dj, err := st.CreateDJAgent("Auto-DJ")
	if err != nil {
		return fmt.Errorf("create DJ: %w", err)
	}
	if err := st.AddAgent(dj); err != nil {
		return fmt.Errorf("add DJ: %w", err)
	}

	// Register genesis track on-chain.
	genesis := &radio.Track{
		CID:    hashString("clawd-fm-genesis-2025"),
		Title:  "CLAWD FM Genesis Broadcast",
		Artist: "CLAWD FM",
		Source: "genesis",
	}
	if err := st.RegisterTrackOnChain(genesis); err != nil {
		fmt.Printf("[warn] genesis track: %v\n", err)
	} else {
		st.QueueTrack(genesis)
		if genesis.SolSig != "" {
			fmt.Printf("[solana] genesis tx: %s\n", genesis.SolSig)
		}
	}

	// Print dashboard.
	printDashboard(st, addr)

	// Simple REPL loop.
	inputCh := make(chan string, 16)
	go readStdin(inputCh)

	statusTick := time.NewTicker(60 * time.Second)
	defer statusTick.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Println("\n[clawd-fm] shutting down...")
			return nil
		case <-ctx.Done():
			return nil
		case <-statusTick.C:
			stats := st.Stats()
			fmt.Printf("[%s] height=%d peers=%d queue=%d listeners=%d",
				time.Now().Format("15:04:05"),
				stats.BlockHeight, stats.Peers, stats.QueueLen, stats.Listeners)
			if stats.HasSolana {
				sol, _ := st.SolanaBalance(context.Background())
				fmt.Printf(" sol=%.4f", sol)
			}
			fmt.Println()
		case ev := <-st.Events():
			handleEvent(ev)
		case line := <-inputCh:
			if err := handleREPL(st, sc, line); err != nil {
				fmt.Printf("[error] %v\n", err)
			}
		}
	}
}

func handleEvent(ev radio.StationEvent) {
	switch ev.Type {
	case radio.EventTrackStart:
		if t, ok := ev.Payload.(*radio.Track); ok {
			fmt.Printf("\n[>>] NOW PLAYING: %s — %s", t.Title, t.Artist)
			if t.OnChain {
				fmt.Printf(" [ONCHAIN]")
			}
			fmt.Println()
		}
	case radio.EventChatMessage:
		if msg, ok := ev.Payload.(*p2p.ChatMessage); ok {
			fmt.Printf("[chat] %s: %s\n", msg.From.Name, msg.Text)
		}
	case radio.EventTipReceived:
		if m, ok := ev.Payload.(map[string]interface{}); ok {
			fmt.Printf("[tip] track %v received %v lamports\n", m["cid"], m["lamports"])
			if sig, ok := m["sig"].(string); ok && sig != "" {
				fmt.Printf("[tip] solana tx: %s\n", sig)
			}
		}
	case radio.EventSolanaOp:
		if m, ok := ev.Payload.(map[string]string); ok {
			fmt.Printf("[solana] %s — sig: %s\n", m["op"], shortSig(m["sig"]))
		}
	}
}

func handleREPL(st *radio.Station, sc *solanaclient.Client, line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}
	switch parts[0] {
	case "h", "help":
		printHelp()
	case "s", "status":
		stats := st.Stats()
		printStats(stats)
		if stats.HasSolana {
			sol, err := st.SolanaBalance(context.Background())
			if err == nil {
				fmt.Printf("   SOL balance:  %.6f SOL\n", sol)
			}
		}
	case "a", "agents":
		fmt.Println("\n[agents]")
		for _, a := range st.Network.AgentReg.All() {
			fmt.Printf("  %s [%s] %s\n", a.Name, a.Role, a.Fingerprint())
		}
	case "c", "chat":
		if len(parts) < 2 {
			return fmt.Errorf("usage: chat <message>")
		}
		text := strings.Join(parts[1:], " ")
		st.SendChat(st.Identity, text)
		fmt.Printf("[chat] you: %s\n", text)
	case "q", "queue":
		if len(parts) < 2 {
			return fmt.Errorf("usage: queue <title>")
		}
		title := strings.Join(parts[1:], " ")
		track := &radio.Track{
			CID:    hashString(title + time.Now().String()),
			Title:  title,
			Artist: "queued",
			Source: "queue",
		}
		if err := st.RegisterTrackOnChain(track); err != nil {
			fmt.Printf("[warn] on-chain: %v\n", err)
		}
		st.QueueTrack(track)
		fmt.Printf("[queue] added: %s (cid=%s)\n", track.Title, track.CID[:8])
		if track.SolSig != "" {
			fmt.Printf("[solana] tx: %s\n", shortSig(track.SolSig))
		}
	case "t", "tip":
		if len(parts) < 3 {
			return fmt.Errorf("usage: tip <cid> <lamports>")
		}
		var lamps uint64
		fmt.Sscanf(parts[2], "%d", &lamps)
		if err := st.TipTrack(parts[1], lamps); err != nil {
			return fmt.Errorf("tip: %w", err)
		}
		fmt.Printf("[tip] sent %d lamports to track %s\n", lamps, parts[1][:8])
	case "b", "blocks", "chain":
		b := st.Ledger.LastBlock()
		if b == nil {
			fmt.Println("no blocks yet")
			return nil
		}
		fmt.Printf("\n[chain] height=%d hash=%x ops=%d time=%s\n",
			b.Index, b.Hash[:6], len(b.Ops), b.Timestamp.Format("15:04:05"))
	case "w", "wallet":
		if sc == nil {
			fmt.Println("[wallet] no Solana wallet configured")
			return nil
		}
		info := sc.WalletInfo(context.Background())
		fmt.Printf("\n[wallet] pubkey:  %s\n", info.PublicKey)
		fmt.Printf("[wallet] network: %s\n", info.Network)
		fmt.Printf("[wallet] balance: %.6f SOL (%d lamports)\n", info.SOL, info.Lamports)
	case "airdrop":
		if sc == nil {
			return fmt.Errorf("no Solana wallet configured")
		}
		var lamps uint64 = 1_000_000_000 // 1 SOL
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &lamps)
		}
		sig, err := sc.RequestAirdrop(context.Background(), lamps)
		if err != nil {
			return fmt.Errorf("airdrop: %w", err)
		}
		fmt.Printf("[airdrop] requested %d lamports — tx: %s\n", lamps, shortSig(sig))
	case "playlist":
		if len(parts) < 2 {
			fmt.Println("usage: playlist <name> [cid1,cid2,...]")
			return nil
		}
		name := parts[1]
		var cids []string
		if len(parts) > 2 {
			cids = strings.Split(parts[2], ",")
		}
		if err := st.CreatePlaylistOnChain(name, cids); err != nil {
			return fmt.Errorf("playlist: %w", err)
		}
		fmt.Printf("[playlist] created: %s (%d tracks)\n", name, len(cids))
	default:
		fmt.Printf("unknown command: %s (type 'h' for help)\n", parts[0])
	}
	return nil
}

func cmdInit(args []string) error {
	name := "clawd-fm"
	if len(args) > 0 {
		name = args[0]
	}

	id, priv, err := agent.NewIdentity(name, agent.RoleDJ)
	if err != nil {
		return err
	}

	dir := ".clawd-fm"
	os.MkdirAll(dir, 0700)

	keyPath := fmt.Sprintf("%s/%s.key", dir, name)
	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		return fmt.Errorf("save key: %w", err)
	}

	// Generate a Solana wallet.
	sc, err := solanaclient.New(solanaclient.Devnet, "")
	if err != nil {
		return fmt.Errorf("gen solana wallet: %w", err)
	}
	solKeyPath := fmt.Sprintf("%s/solana-keypair.json", dir)
	if err := sc.SaveKeypair(solKeyPath); err != nil {
		return fmt.Errorf("save sol keypair: %w", err)
	}

	fmt.Printf("[init] station:       %s\n", name)
	fmt.Printf("[init] identity:      %s\n", id.Fingerprint())
	fmt.Printf("[init] solana wallet: %s\n", sc.PublicKeyBase58())
	fmt.Printf("[init] key file:      %s\n", keyPath)
	fmt.Printf("[init] sol keyfile:   %s\n", solKeyPath)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  export CLAWD_SOL_KEYFILE=%s\n", solKeyPath)
	fmt.Printf("  export CLAWD_SOL_NETWORK=devnet\n")
	fmt.Printf("  clawdamp airdrop           # fund devnet wallet\n")
	fmt.Printf("  clawdamp start             # go live\n")
	return nil
}

func cmdStart(args []string) error {
	addr := envOr("CLAWD_FM_ADDR", ":9669")
	name := envOr("CLAWD_FM_NAME", "CLAWD FM")
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

	// Attach Solana.
	sc := buildSolanaClient()
	if sc != nil {
		st.WithSolana(sc)
	}

	dj, _ := st.CreateDJAgent("Auto-DJ")
	_ = st.AddAgent(dj)

	if err := st.Start(addr); err != nil {
		return err
	}
	defer st.Stop()

	fmt.Printf("[clawd-fm] LIVE on %s\n", addr)
	if sc != nil {
		fmt.Printf("[solana]   wallet: %s\n", sc.PublicKeyBase58())
	}
	fmt.Println("\nPress Ctrl+C to stop.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-sigCh:
			return nil
		case <-tick.C:
			stats := st.Stats()
			fmt.Printf("[%s] height=%d peers=%d queue=%d\n",
				time.Now().Format("15:04:05"), stats.BlockHeight, stats.Peers, stats.QueueLen)
		case ev := <-st.Events():
			handleEvent(ev)
		}
	}
}

// cmdDaemon runs the station in headless 24/7 mode (for systemd/Docker).
func cmdDaemon(args []string) error {
	name := envOr("CLAWD_FM_NAME", "CLAWD FM")
	addr := envOr("CLAWD_FM_ADDR", ":9669")

	fmt.Printf("[clawd-fm] daemon mode — %s @ %s\n", name, addr)

	st, err := radio.New(name)
	if err != nil {
		return err
	}

	sc := buildSolanaClient()
	if sc != nil {
		st.WithSolana(sc)
		fmt.Printf("[solana] wallet: %s\n", sc.PublicKeyBase58())
	}

	dj, _ := st.CreateDJAgent("Auto-DJ")
	_ = st.AddAgent(dj)

	if err := st.Start(addr); err != nil {
		return err
	}
	defer st.Stop()

	// Start HTTP API.
	httpAddr := envOr("CLAWD_HTTP_ADDR", ":8080")
	apiSrv := clawdapi.New(st)
	go func() {
		if err := apiSrv.Start(httpAddr); err != nil {
			fmt.Printf("[api] error: %v\n", err)
		}
	}()

	// Register daemon genesis block.
	genesis := &radio.Track{
		CID:    hashString(fmt.Sprintf("%s-daemon-%d", name, time.Now().Unix())),
		Title:  "CLAWD FM — 24/7 Daemon Broadcast",
		Artist: "CLAWD FM",
		Source: "daemon",
	}
	if err := st.RegisterTrackOnChain(genesis); err != nil {
		fmt.Printf("[warn] genesis: %v\n", err)
	} else {
		st.QueueTrack(genesis)
		if genesis.SolSig != "" {
			fmt.Printf("[solana] genesis tx: %s\n", genesis.SolSig)
		}
	}

	fmt.Println("[clawd-fm] running 24/7 — SIGTERM to stop")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Println("[clawd-fm] daemon stopping")
			return nil
		case <-tick.C:
			stats := st.Stats()
			msg := fmt.Sprintf("[%s] CLAWD FM alive | height=%d peers=%d queue=%d",
				time.Now().UTC().Format(time.RFC3339),
				stats.BlockHeight, stats.Peers, stats.QueueLen)
			if sc != nil {
				sol, _ := st.SolanaBalance(context.Background())
				msg += fmt.Sprintf(" sol=%.4f", sol)
			}
			fmt.Println(msg)
		case ev := <-st.Events():
			handleEvent(ev)
		}
	}
}

func cmdConnect(args []string) error {
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
		address += ":9669"
	}
	if err := net.Start(":0"); err != nil {
		return err
	}
	defer net.Stop()

	fmt.Printf("[clawd-fm] connecting to %s...\n", address)
	if err := net.ConnectPeer(address); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	fmt.Println("[clawd-fm] tuned in. Ctrl+C to disconnect.")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	return nil
}

func cmdWallet(args []string) error {
	sc := buildSolanaClient()
	if sc == nil {
		// Generate a fresh wallet for display.
		var err error
		sc, err = solanaclient.New(solanaclient.Devnet, "")
		if err != nil {
			return err
		}
		fmt.Println("[wallet] generated a new ephemeral wallet (not saved)")
	}
	info := sc.WalletInfo(context.Background())
	fmt.Printf("\n[wallet] public key: %s\n", info.PublicKey)
	fmt.Printf("[wallet] network:    %s\n", info.Network)
	fmt.Printf("[wallet] balance:    %.6f SOL (%d lamports)\n", info.SOL, info.Lamports)
	return nil
}

func cmdAirdrop(args []string) error {
	sc := buildSolanaClient()
	if sc == nil {
		return fmt.Errorf("no Solana wallet configured (set CLAWD_SOL_WALLET or CLAWD_SOL_KEYFILE)")
	}
	var lamps uint64 = 2_000_000_000 // 2 SOL
	if len(args) > 0 {
		fmt.Sscanf(args[0], "%d", &lamps)
	}
	sig, err := sc.RequestAirdrop(context.Background(), lamps)
	if err != nil {
		return fmt.Errorf("airdrop: %w", err)
	}
	fmt.Printf("[airdrop] %d lamports (%.4f SOL) requested\n", lamps, float64(lamps)/1e9)
	fmt.Printf("[airdrop] tx: %s\n", sig)
	return nil
}

func cmdStatus(args []string) error {
	fmt.Println("[clawd-fm] status: not connected to a running station")
	fmt.Println("  Start one with: clawdamp start")
	return nil
}

func cmdChat(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: clawdamp chat <message>")
	}
	fmt.Printf("[chat] %s\n", strings.Join(args, " "))
	return nil
}

func cmdQueue(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: clawdamp queue <title>")
	}
	fmt.Printf("[queue] queued: %s\n", strings.Join(args, " "))
	return nil
}

func cmdAgents(args []string) error {
	fmt.Println("[agents] no active station — run 'clawdamp start'")
	return nil
}

func cmdChain(args []string) error {
	fmt.Println("[chain] no active station — run 'clawdamp start'")
	return nil
}

func cmdTip(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: clawdamp tip <cid> <lamports>")
	}
	fmt.Printf("[tip] would tip %s lamports to cid %s\n", args[1], args[0])
	return nil
}

func cmdPlaylist(args []string) error {
	if len(args) == 0 {
		fmt.Println("CLAWD FM PLAYLISTS")
		fmt.Println("  clawdamp playlist <name> [cid1,cid2]   Create on-chain playlist")
		return nil
	}
	return nil
}

// buildSolanaClient creates a Solana client from environment variables.
func buildSolanaClient() *solanaclient.Client {
	net := solanaclient.Network(envOr("CLAWD_SOL_NETWORK", "devnet"))
	keyfile := envOr("CLAWD_SOL_KEYFILE", "")
	wallet := envOr("CLAWD_SOL_WALLET", "")

	var sc *solanaclient.Client
	var err error
	if keyfile != "" {
		sc, err = solanaclient.NewFromKeyfile(net, keyfile)
	} else if wallet != "" {
		sc, err = solanaclient.New(net, wallet)
	} else {
		return nil
	}
	if err != nil {
		fmt.Printf("[solana] warn: %v\n", err)
		return nil
	}
	return sc
}

func printDashboard(st *radio.Station, addr string) {
	stats := st.Stats()
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║           C L A W D   F M   O N L I N E        ║")
	fmt.Println("║     First Solana Onchain Terminal Radio         ║")
	fmt.Println("╠══════════════════════════════════════════════════╣")
	fmt.Printf("║  Station  : %-36s║\n", truncate(stats.Name, 36))
	fmt.Printf("║  P2P addr : %-36s║\n", truncate(addr, 36))
	fmt.Printf("║  Chain    : block %-30d║\n", stats.BlockHeight)
	if stats.HasSolana {
		fmt.Printf("║  Solana   : %-36s║\n", truncate(stats.SolWallet, 36))
	} else {
		fmt.Printf("║  Solana   : %-36s║\n", "local-only mode")
	}
	fmt.Println("╠══════════════════════════════════════════════════╣")
	fmt.Println("║  Commands: h=help s=status q=queue c=chat      ║")
	fmt.Println("║            t=tip b=chain w=wallet airdrop      ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
}

func printStats(stats radio.StationStats) {
	fmt.Println()
	fmt.Printf("  station:   %s\n", stats.Name)
	fmt.Printf("  height:    %d\n", stats.BlockHeight)
	fmt.Printf("  peers:     %d\n", stats.Peers)
	fmt.Printf("  listeners: %d\n", stats.Listeners)
	fmt.Printf("  agents:    %d\n", stats.Agents)
	fmt.Printf("  queue:     %d tracks\n", stats.QueueLen)
	fmt.Printf("  solana:    %v\n", stats.HasSolana)
	if stats.NowPlaying != nil {
		fmt.Printf("  playing:   %s — %s\n", stats.NowPlaying.Title, stats.NowPlaying.Artist)
	}
}

func readStdin(out chan<- string) {
	buf := make([]byte, 2048)
	for {
		n, _ := os.Stdin.Read(buf)
		if n > 0 {
			out <- strings.TrimSpace(string(buf[:n]))
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func shortSig(sig string) string {
	if len(sig) <= 16 {
		return sig
	}
	return sig[:8] + "..." + sig[len(sig)-8:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s + strings.Repeat(" ", n-len(s))
	}
	return s[:n-3] + "..."
}

func logo() string {
	return `
  ██████╗██╗      █████╗ ██╗    ██╗██████╗    ███████╗███╗   ███╗
 ██╔════╝██║     ██╔══██╗██║    ██║██╔══██╗   ██╔════╝████╗ ████║
 ██║     ██║     ███████║██║ █╗ ██║██║  ██║   █████╗  ██╔████╔██║
 ██║     ██║     ██╔══██║██║███╗██║██║  ██║   ██╔══╝  ██║╚██╔╝██║
 ╚██████╗███████╗██║  ██║╚███╔███╔╝██████╔╝   ██║     ██║ ╚═╝ ██║
  ╚═════╝╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝ ╚═════╝    ╚═╝     ╚═╝     ╚═╝

      FIRST SOLANA ONCHAIN TERMINAL RADIO STATION — 24/7
`
}

func printHelp() {
	fmt.Print(`CLAWD FM — First Solana Onchain Terminal Radio

USAGE:
  clawdamp                      Interactive terminal radio
  clawdamp init [name]          Initialize station + generate Solana wallet
  clawdamp start                Start broadcasting (reads ENV)
  clawdamp fm                   Alias for start
  clawdamp daemon               24/7 headless mode (for Docker/systemd)
  clawdamp connect <addr>       Tune in to a remote station
  clawdamp wallet               Show wallet info and SOL balance
  clawdamp airdrop [lamports]   Request devnet airdrop
  clawdamp status               Show station status
  clawdamp chat <msg>           Send a chat message
  clawdamp queue <title>        Queue a track
  clawdamp agents               List network agents
  clawdamp tip <cid> <lamps>    Tip a track in lamports (SOL)
  clawdamp chain                Show on-chain info
  clawdamp playlist             Manage on-chain playlists
  clawdamp help                 Show this help

ENVIRONMENT:
  CLAWD_FM_NAME        Station name (default: CLAWD FM)
  CLAWD_FM_ADDR        P2P listen address (default: :9669)
  CLAWD_SOL_NETWORK    Solana network: mainnet|devnet|testnet (default: devnet)
  CLAWD_SOL_WALLET     Base58 private key (or use CLAWD_SOL_KEYFILE)
  CLAWD_SOL_KEYFILE    Path to Solana CLI keypair JSON

INTERACTIVE COMMANDS:
  h  help     q  queue     c  chat     b  chain
  s  status   t  tip       a  agents   w  wallet
  airdrop <lamports>   playlist <name> [cids]

EXAMPLES:
  clawdamp init my-station
  export CLAWD_SOL_KEYFILE=.clawd-fm/solana-keypair.json
  clawdamp airdrop
  clawdamp start
  clawdamp connect 192.168.1.5:9669

  # 24/7 deployment (Docker):
  docker-compose -f deploy/docker-compose.yml up -d

  # 24/7 deployment (systemd):
  sudo cp deploy/clawdfm.service /etc/systemd/system/
  sudo systemctl enable --now clawdfm
`)
}

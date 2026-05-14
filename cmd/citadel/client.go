package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vipul0092/citadel/internal/client"
	"github.com/vipul0092/citadel/internal/control"
	"github.com/vipul0092/citadel/internal/discovery"
)

func runClient(args []string) {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	serverAddr := fs.String("server", "", "skip mDNS; connect to host:port directly")
	preFillName := fs.String("name", "", "pre-fill the name prompt")
	headless := fs.Bool("headless", false, "connect without TUI; writes ~/.citadel/client/current.json")
	_ = fs.Parse(args)

	if *headless {
		runClientHeadless(*serverAddr, *preFillName, versionString())
		return
	}

	var scanCh <-chan discovery.ServerInfo
	var cancelScan context.CancelFunc

	if *serverAddr == "" {
		ctx, cancel := context.WithCancel(context.Background())
		cancelScan = cancel
		// Run mDNS and UDP broadcast in parallel; merge into one deduplicated stream.
		scanCh = discovery.Merge(ctx, discovery.Browse(ctx), discovery.BrowseBroadcast(ctx))
	}

	// Silence slog while the TUI owns the terminal — same as server TUI path.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	tui := client.NewTUI(*serverAddr, *preFillName, scanCh, versionString())
	p := tea.NewProgram(tui, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
	}

	if cancelScan != nil {
		cancelScan()
	}
}

// runClientHeadless dials a server, completes the handshake (which starts the control plane),
// writes ~/.citadel/client/current.json, and blocks until SIGINT/SIGTERM.
func runClientHeadless(serverAddr, name, version string) {
	if serverAddr == "" {
		fmt.Fprintln(os.Stderr, "error: --server is required in headless mode")
		os.Exit(1)
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, "error: --name is required in headless mode")
		os.Exit(1)
	}

	conn, err := client.Dial(serverAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: connect to %s: %v\n", serverAddr, err)
		os.Exit(1)
	}

	if err := conn.Handshake(name, version); err != nil {
		fmt.Fprintf(os.Stderr, "error: handshake: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	slog.Info("connected headless",
		"name", conn.Name(),
		"server", conn.ServerName,
		"addr", serverAddr,
		"sock", conn.ControlSockPath(),
	)

	ptr, err := control.WriteClientPointer(
		conn.ControlSockPath(),
		conn.Addr(),
		conn.ServerName,
		conn.Name(),
	)
	if err != nil {
		slog.Warn("client pointer write failed", "err", err)
	}
	defer ptr.Unlink()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	slog.Info("shutting down")
}

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vipul0092/citadel/internal/server"
)

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	name := fs.String("name", "", "server display name (required)")
	port := fs.Int("port", 7777, "TCP listen port")
	motd := fs.String("motd", "", "message of the day")
	maxClients := fs.Int("max-clients", 16, "max simultaneous clients")
	logFile := fs.String("log-file", "", "activity log path (default ~/.citadel/<name>/activity.log)")
	_ = fs.Parse(args)

	if *name == "" {
		fmt.Fprintln(os.Stderr, "error: --name is required")
		os.Exit(1)
	}

	resolvedLog := *logFile
	if resolvedLog == "" {
		home, _ := os.UserHomeDir()
		resolvedLog = filepath.Join(home, ".citadel", *name, "activity.log")
	}

	cfg := server.Config{
		Name:       *name,
		Port:       *port,
		Motd:       *motd,
		MaxClients: *maxClients,
		LogFile:    resolvedLog,
	}

	srv, err := server.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Compute listen address before starting (avoids race with srv.ListenAddr())
	listenAddr := net.JoinHostPort(server.LocalIPv4(), fmt.Sprintf("%d", *port))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run(ctx) }()

	// Silence slog while the TUI owns the terminal — the TUI shows all events.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	tui := server.NewTUI(srv.Hub(), *name, listenAddr, versionString())
	p := tea.NewProgram(tui, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
	}

	cancel()
	if err := <-errCh; err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

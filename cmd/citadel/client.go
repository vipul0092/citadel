package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vipul0092/citadel/internal/client"
	"github.com/vipul0092/citadel/internal/discovery"
)

func runClient(args []string) {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	serverAddr := fs.String("server", "", "skip mDNS; connect to host:port directly")
	preFillName := fs.String("name", "", "pre-fill the name prompt")
	_ = fs.Parse(args)

	var scanCh <-chan discovery.ServerInfo
	var cancelScan context.CancelFunc

	if *serverAddr == "" {
		ctx, cancel := context.WithCancel(context.Background())
		cancelScan = cancel
		// Run mDNS and UDP broadcast in parallel; merge into one deduplicated stream.
		scanCh = discovery.Merge(ctx, discovery.Browse(ctx), discovery.BrowseBroadcast(ctx))
	}

	tui := client.NewTUI(*serverAddr, *preFillName, scanCh, versionString())
	p := tea.NewProgram(tui, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}

	if cancelScan != nil {
		cancelScan()
	}
}

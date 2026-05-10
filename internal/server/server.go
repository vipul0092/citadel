package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/vipul0092/citadel/internal/discovery"
)

// Config holds server startup parameters.
type Config struct {
	Name       string
	Port       int
	Motd       string
	MaxClients int
	LogFile    string
}

// Server manages the TCP listener and hub lifecycle.
type Server struct {
	cfg      Config
	hub      *Hub
	log      *ActivityLog
	listener net.Listener
}

// New creates a Server. Call Run to start serving.
func New(cfg Config) (*Server, error) {
	actLog, err := OpenActivityLog(cfg.LogFile)
	if err != nil {
		return nil, fmt.Errorf("opening activity log: %w", err)
	}
	hub := NewHub(cfg.MaxClients, cfg.Motd, actLog)
	return &Server{cfg: cfg, hub: hub, log: actLog}, nil
}

// Hub returns the hub so the TUI can subscribe to events and dispatch commands.
func (s *Server) Hub() *Hub { return s.hub }

// Run starts the TCP listener, mDNS advertise, and the hub goroutine.
// It blocks until ctx is cancelled, then shuts down cleanly.
func (s *Server) Run(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	s.listener = ln

	localIP := LocalIPv4()
	slog.Info("server listening", "name", s.cfg.Name, "addr", net.JoinHostPort(localIP, fmt.Sprintf("%d", s.cfg.Port)))

	if err := discovery.Advertise(ctx, s.cfg.Name, s.cfg.Port); err != nil {
		slog.Warn("mDNS advertise failed", "err", err)
	}
	discovery.BroadcastPresence(ctx, s.cfg.Name, s.cfg.Port, LocalIPv4())

	go s.hub.Run()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down server")
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				s.log.Close()
				return nil
			default:
				slog.Error("accept error", "err", err)
				continue
			}
		}
		c := newClientConn(conn, s.cfg.Name, s.hub)
		go c.serve()
	}
}

// LocalIPv4 returns the machine's outbound IPv4 address.
func LocalIPv4() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer func() { _ = conn.Close() }()
	host, _, _ := net.SplitHostPort(conn.LocalAddr().String())
	return host
}

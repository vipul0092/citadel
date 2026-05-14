package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"

	"github.com/vipul0092/citadel/internal/control"
	"github.com/vipul0092/citadel/internal/discovery"
)

// Config holds server startup parameters.
type Config struct {
	Name       string
	Port       int
	Motd       string
	MaxClients int
	LogFile    string
	Version    string // displayed in control-plane hello frames
}

// Server manages the TCP listener and hub lifecycle.
type Server struct {
	cfg        Config
	hub        *Hub
	log        *ActivityLog
	listener   net.Listener
	ctrl       *control.Plane
	listenAddr string      // set in Run() after bind; safe after bindReady is closed
	bindReady  chan struct{} // closed after net.Listen succeeds
}

// New creates a Server. Call Run to start serving.
func New(cfg Config) (*Server, error) {
	actLog, err := OpenActivityLog(cfg.LogFile)
	if err != nil {
		return nil, fmt.Errorf("opening activity log: %w", err)
	}
	hub := NewHub(cfg.MaxClients, cfg.Motd, actLog)
	return &Server{cfg: cfg, hub: hub, log: actLog, bindReady: make(chan struct{})}, nil
}

// Hub returns the hub so the TUI can subscribe to events and dispatch commands.
func (s *Server) Hub() *Hub { return s.hub }

// WaitListenAddr blocks until net.Listen has bound and returns the actual listen
// address (including the OS-assigned port when --port 0 is used).
func (s *Server) WaitListenAddr(ctx context.Context) (string, error) {
	select {
	case <-s.bindReady:
		return s.listenAddr, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Run starts the TCP listener, mDNS advertise, and the hub goroutine.
// It blocks until ctx is cancelled, then shuts down cleanly.
func (s *Server) Run(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	s.listener = ln

	actualPort := ln.Addr().(*net.TCPAddr).Port
	localIP := LocalIPv4()
	listenAddr := net.JoinHostPort(localIP, fmt.Sprintf("%d", actualPort))
	s.listenAddr = listenAddr
	close(s.bindReady) // unblock WaitListenAddr callers
	slog.Info("server listening", "name", s.cfg.Name, "addr", listenAddr)

	ctrl, err := control.New("server", s.cfg.Name, listenAddr, s.cfg.Version, s.cfg.Name, NewActionsProvider(s.hub))
	if err != nil {
		slog.Warn("control plane start failed", "err", err)
	} else {
		s.ctrl = ctrl
		go s.bridgeHubEvents()
	}

	if err := discovery.Advertise(ctx, s.cfg.Name, s.cfg.Port); err != nil {
		slog.Warn("mDNS advertise failed", "err", err)
	}
	discovery.BroadcastPresence(ctx, s.cfg.Name, s.cfg.Port, LocalIPv4())

	go s.hub.Run()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down server")
		_ = ln.Close()
		if s.ctrl != nil {
			s.ctrl.Close()
		}
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

// bridgeHubEvents reads HubEvents from the control-events channel and forwards them
// to the control-plane hub as typed control events.
func (s *Server) bridgeHubEvents() {
	for ev := range s.hub.ControlEvents {
		if s.ctrl == nil {
			return
		}
		h := s.ctrl.Hub()
		switch ev.Kind {
		case EvJoin:
			data, _ := json.Marshal(struct {
				Name string `json:"name"`
			}{Name: ev.Name})
			h.Emit("peer-join", data)

		case EvLeave:
			data, _ := json.Marshal(struct {
				Name string `json:"name"`
			}{Name: ev.Name})
			h.Emit("peer-leave", data)

		case EvKick:
			data, _ := json.Marshal(struct {
				Name   string `json:"name"`
				Reason string `json:"reason"`
			}{Name: ev.Name, Reason: ev.Text})
			h.Emit("kick", data)

		case EvChat:
			data, _ := json.Marshal(struct {
				Name string `json:"name"`
				Text string `json:"text"`
			}{Name: ev.Name, Text: ev.Text})
			h.Emit("chat", data)

		case EvDirect:
			data, _ := json.Marshal(struct {
				Name string `json:"name"`
				To   string `json:"to"`
				Text string `json:"text"`
			}{Name: ev.Name, To: ev.Target, Text: ev.Text})
			h.Emit("chat-direct", data)

		case EvSay:
			data, _ := json.Marshal(struct {
				Text string `json:"text"`
			}{Text: ev.Text})
			h.Emit("say", data)

		case EvMotd:
			data, _ := json.Marshal(struct {
				Text string `json:"text"`
			}{Text: ev.Text})
			h.Emit("motd-changed", data)
		}
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

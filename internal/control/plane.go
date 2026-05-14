package control

import (
	"log/slog"
	"os"
)

// Plane is the top-level control-plane handle for one citadel process.
// It ties the hub, UDS listener, and sentinel file together.
type Plane struct {
	hub      *Hub
	sentinel *Sentinel
	sockPath string
	stop     func()
}

// New opens the UDS listener, writes the sentinel file, and returns a ready Plane.
//
//   - role:    "server" or "client"
//   - name:    server/session name displayed in the dashboard
//   - addr:    externally-reachable TCP address (e.g. "192.168.1.5:7777")
//   - version: version string for hello frames
//   - actions: callbacks for action ops (kick, say, send-chat, etc.); nil = ENOTSUP for all
func New(role, name, addr, version, serverName string, actions ActionsProvider) (*Plane, error) {
	hub := NewHub()

	sockPath, stop, err := openListener(hub, role, name, version, actions)
	if err != nil {
		return nil, err
	}

	sentinel, err := WriteSentinel(role, name, addr, sockPath, serverName, os.Args[1:])
	if err != nil {
		stop()
		return nil, err
	}

	slog.Debug("control plane started", "role", role, "name", name, "sock", sockPath)
	return &Plane{hub: hub, sentinel: sentinel, sockPath: sockPath, stop: stop}, nil
}

// SockPath returns the UDS socket path for this process's control plane.
func (p *Plane) SockPath() string { return p.sockPath }

// Hub returns the Hub that server/client code should call Emit/EmitGame/ForwardEnvelope on.
func (p *Plane) Hub() *Hub {
	return p.hub
}

// Close shuts down the control plane: sends bye to all subscribers, stops the listener,
// and unlinks the sentinel file.
func (p *Plane) Close() {
	p.hub.SendBye("shutdown")
	p.stop()
	p.sentinel.Unlink()
	slog.Debug("control plane stopped")
}

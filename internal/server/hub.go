package server

import (
	"bytes"
	"log/slog"
	"time"

	"github.com/vipul0092/citadel/internal/proto"
)

// HubEventKind identifies the type of a hub event emitted to the TUI.
type HubEventKind int

const (
	EvJoin HubEventKind = iota
	EvLeave
	EvKick
	EvChat
	EvDirect
	EvSay
	EvMotd
)

// HubEvent carries a state change notification to the server TUI.
type HubEvent struct {
	Kind    HubEventKind
	Name    string
	Target  string
	Text    string
	Peers   []PeerEntry
}

// PeerEntry describes one connected client for display in the TUI.
type PeerEntry struct {
	Name      string
	RemoteIP  string
	Connected time.Time
}

// registerReq is the atomic check-and-register request sent by a new clientConn.
type registerReq struct {
	conn   *clientConn
	result chan registerResult
}

// registerResult carries the server's response to a registerReq.
type registerResult struct {
	OK     bool
	Reason string   // only when !OK
	Peers  []string // only when OK
	Motd   string   // only when OK
}

type broadcastMsg struct {
	data    []byte
	exclude string
}

type directMsg struct {
	to   string
	data []byte
}

type kickReq struct {
	name   string
	reason string
	result chan bool
}

// Hub is the central message router. All map mutations happen in its Run goroutine.
type Hub struct {
	regCh    chan registerReq
	unregCh  chan *clientConn
	bcastCh  chan broadcastMsg
	directCh chan directMsg
	kickCh   chan kickReq
	sayCh    chan string
	motdCh   chan string

	Events chan HubEvent // consumed by server TUI; non-blocking send

	// state owned exclusively by Run goroutine
	clients    map[string]*clientConn
	motd       string
	maxClients int
	log        *ActivityLog
}

// NewHub creates a Hub ready to be started with Run.
func NewHub(maxClients int, motd string, log *ActivityLog) *Hub {
	return &Hub{
		regCh:      make(chan registerReq, 8),
		unregCh:    make(chan *clientConn, 32),
		bcastCh:    make(chan broadcastMsg, 128),
		directCh:   make(chan directMsg, 128),
		kickCh:     make(chan kickReq, 8),
		sayCh:      make(chan string, 8),
		motdCh:     make(chan string, 8),
		Events:     make(chan HubEvent, 64),
		clients:    make(map[string]*clientConn),
		motd:       motd,
		maxClients: maxClients,
		log:        log,
	}
}

// Run processes all hub commands. Call in a dedicated goroutine; runs until the process exits.
func (h *Hub) Run() {
	for {
		select {
		case req := <-h.regCh:
			h.handleRegister(req)

		case c := <-h.unregCh:
			h.handleUnregister(c)

		case msg := <-h.bcastCh:
			for _, c := range h.clients {
				if c.name != msg.exclude {
					c.enqueue(msg.data)
				}
			}

		case msg := <-h.directCh:
			if c, ok := h.clients[msg.to]; ok {
				c.enqueue(msg.data)
			}

		case req := <-h.kickCh:
			h.handleKick(req)

		case text := <-h.sayCh:
			data := h.encode(proto.TypeChat, "server", "", proto.ChatPayload{Text: text})
			for _, c := range h.clients {
				c.enqueue(data)
			}
			h.log.Say(text)
			h.emit(HubEvent{Kind: EvSay, Text: text})

		case motd := <-h.motdCh:
			h.motd = motd
			data := h.encode(proto.TypeSystem, "", "", proto.SystemPayload{
				Event:   proto.EvMotd,
				Message: motd,
			})
			for _, c := range h.clients {
				c.enqueue(data)
			}
			h.log.Motd(motd)
			h.emit(HubEvent{Kind: EvMotd, Text: motd})
		}
	}
}

func (h *Hub) handleRegister(req registerReq) {
	name := req.conn.name
	if _, exists := h.clients[name]; exists {
		req.result <- registerResult{OK: false, Reason: "name taken"}
		return
	}
	if len(h.clients) >= h.maxClients {
		req.result <- registerResult{OK: false, Reason: "server full"}
		return
	}
	h.clients[name] = req.conn
	peers := h.peerNames(name)
	req.result <- registerResult{OK: true, Peers: peers, Motd: h.motd}

	// broadcast join event to all other clients
	data := h.encode(proto.TypeSystem, "", "", proto.SystemPayload{
		Event: proto.EvJoin,
		Name:  name,
	})
	for _, c := range h.clients {
		if c.name != name {
			c.enqueue(data)
		}
	}
	h.log.Join(name)
	h.emit(HubEvent{Kind: EvJoin, Name: name, Peers: h.peerSnapshot()})
	slog.Info("client joined", "name", name)
}

func (h *Hub) handleUnregister(c *clientConn) {
	if _, ok := h.clients[c.name]; !ok {
		return // already removed (e.g. by kick)
	}
	delete(h.clients, c.name)
	data := h.encode(proto.TypeSystem, "", "", proto.SystemPayload{
		Event: proto.EvLeave,
		Name:  c.name,
	})
	for _, peer := range h.clients {
		peer.enqueue(data)
	}
	h.log.Leave(c.name)
	h.emit(HubEvent{Kind: EvLeave, Name: c.name, Peers: h.peerSnapshot()})
	slog.Info("client left", "name", c.name)
}

func (h *Hub) handleKick(req kickReq) {
	c, ok := h.clients[req.name]
	if !ok {
		req.result <- false
		return
	}
	kickData := h.encode(proto.TypeKick, "", req.name, proto.KickPayload{Reason: req.reason})
	c.enqueue(kickData)
	c.closeConn()
	delete(h.clients, req.name)

	sysData := h.encode(proto.TypeSystem, "", "", proto.SystemPayload{
		Event:   proto.EvKick,
		Name:    req.name,
		Message: req.reason,
	})
	for _, peer := range h.clients {
		peer.enqueue(sysData)
	}
	h.log.Kick(req.name, req.reason)
	h.emit(HubEvent{Kind: EvKick, Name: req.name, Text: req.reason, Peers: h.peerSnapshot()})
	req.result <- true
}

// Register performs the atomic name-check and registers the conn with the hub.
// Blocks until the hub processes the request.
func (h *Hub) Register(c *clientConn) registerResult {
	result := make(chan registerResult, 1)
	h.regCh <- registerReq{conn: c, result: result}
	return <-result
}

// Unregister removes c from the hub (safe to call after kick or disconnect).
func (h *Hub) Unregister(c *clientConn) {
	h.unregCh <- c
}

// Broadcast encodes and sends payload to all clients except exclude.
func (h *Hub) Broadcast(msgType, from, exclude string, payload any) {
	h.bcastCh <- broadcastMsg{data: h.encode(msgType, from, "", payload), exclude: exclude}
}

// BroadcastRaw sends pre-encoded frame bytes to all clients except exclude.
func (h *Hub) BroadcastRaw(data []byte, exclude string) {
	h.bcastCh <- broadcastMsg{data: data, exclude: exclude}
}

// Direct sends pre-encoded frame bytes to one named client.
func (h *Hub) Direct(to string, data []byte) {
	h.directCh <- directMsg{to: to, data: data}
}

// Kick removes the named client with the given reason. Returns true if the client was found.
func (h *Hub) Kick(name, reason string) bool {
	result := make(chan bool, 1)
	h.kickCh <- kickReq{name: name, reason: reason, result: result}
	return <-result
}

// Say broadcasts a message from the server to all connected clients.
func (h *Hub) Say(text string) {
	h.sayCh <- text
}

// SetMotd updates the server's message of the day and notifies all connected clients.
func (h *Hub) SetMotd(motd string) {
	h.motdCh <- motd
}

// peerNames returns the names of all connected clients except exclude.
func (h *Hub) peerNames(exclude string) []string {
	names := make([]string, 0, len(h.clients))
	for name := range h.clients {
		if name != exclude {
			names = append(names, name)
		}
	}
	return names
}

// peerSnapshot returns PeerEntry for all currently connected clients.
func (h *Hub) peerSnapshot() []PeerEntry {
	entries := make([]PeerEntry, 0, len(h.clients))
	for _, c := range h.clients {
		entries = append(entries, PeerEntry{
			Name:      c.name,
			RemoteIP:  c.remoteIP,
			Connected: c.connectedAt,
		})
	}
	return entries
}

func (h *Hub) encode(msgType, from, to string, payload any) []byte {
	var buf bytes.Buffer
	if err := proto.Encode(&buf, msgType, from, to, payload); err != nil {
		slog.Error("hub encode failed", "type", msgType, "err", err)
		return nil
	}
	return buf.Bytes()
}

func (h *Hub) emit(ev HubEvent) {
	select {
	case h.Events <- ev:
	default: // TUI is behind; drop rather than block the hub
	}
}

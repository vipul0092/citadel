package server

import (
	"encoding/json"
	"sync"
	"time"

	ctrlclient "github.com/vipul0092/citadel/internal/control/client"
)

// HubEventSource is the abstraction the server TUI uses to receive events and dispatch
// admin commands. Two implementations:
//   - inProcessSource: wraps *Hub for the normal in-process TUI.
//   - remoteSource: backed by a control-plane connection for dashboard drill-in.
type HubEventSource interface {
	Events() <-chan HubEvent
	Kick(name, reason string) bool
	Say(text string)
	SetMotd(text string)
	Peers() []PeerEntry
}

// --- in-process source ---

type inProcessSource struct{ hub *Hub }

// NewInProcessSource wraps a *Hub as a HubEventSource.
func NewInProcessSource(h *Hub) HubEventSource { return &inProcessSource{hub: h} }

func (s *inProcessSource) Events() <-chan HubEvent       { return s.hub.Events }
func (s *inProcessSource) Kick(name, reason string) bool { return s.hub.Kick(name, reason) }
func (s *inProcessSource) Say(text string)               { s.hub.Say(text) }
func (s *inProcessSource) SetMotd(text string)           { s.hub.SetMotd(text) }
func (s *inProcessSource) Peers() []PeerEntry            { return s.hub.Peers() }

// --- remote source ---

type remoteSource struct {
	conn     *ctrlclient.Conn
	eventsCh chan HubEvent
	mu       sync.Mutex
	peers    []PeerEntry
}

// NewRemoteSource dials the server's control socket, subscribes at full level,
// and returns a HubEventSource that translates control-plane events to HubEvents.
func NewRemoteSource(sockPath, name, addr string) (HubEventSource, error) {
	conn, err := ctrlclient.Dial(sockPath)
	if err != nil {
		return nil, err
	}
	_ = conn.Send(map[string]any{"op": "subscribe", "level": "full", "since": 0})

	s := &remoteSource{
		conn:     conn,
		eventsCh: make(chan HubEvent, 64),
	}
	go s.translate()
	return s, nil
}

func (s *remoteSource) Events() <-chan HubEvent { return s.eventsCh }

func (s *remoteSource) Kick(name, reason string) bool {
	_ = s.conn.Send(map[string]any{"op": "kick", "name": name, "reason": reason})
	return true // optimistic
}

func (s *remoteSource) Say(text string) {
	_ = s.conn.Send(map[string]any{"op": "say", "text": text})
}

func (s *remoteSource) SetMotd(text string) {
	_ = s.conn.Send(map[string]any{"op": "set-motd", "text": text})
}

func (s *remoteSource) Peers() []PeerEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]PeerEntry{}, s.peers...)
}

func (s *remoteSource) translate() {
	peersSynced := false
	for frame := range s.conn.Events() {
		var m map[string]any
		if err := json.Unmarshal(frame, &m); err != nil {
			continue
		}
		// The "live" marker signals the end of the subscription replay.
		// Request the canonical peer list now so it doesn't race with replay events.
		if m["ev"] == "live" {
			_ = s.conn.Send(map[string]any{"op": "list-peers"})
			peersSynced = true
			continue
		}
		// During replay (before "live"), peer-join/leave events build up s.peers.
		// After list-peers resets the canonical state, skip stale replay peer mutations.
		ev := s.frameToHubEvent(m, peersSynced)
		if ev != nil {
			s.eventsCh <- *ev
		}
	}
	close(s.eventsCh)
}

func (s *remoteSource) frameToHubEvent(m map[string]any, live bool) *HubEvent {
	evType, _ := m["ev"].(string)
	name, _ := m["name"].(string)
	text, _ := m["text"].(string)
	to, _ := m["to"].(string)
	reason, _ := m["reason"].(string)

	switch evType {
	case "peers":
		peers := s.parsePeers(m)
		s.mu.Lock()
		s.peers = peers
		s.mu.Unlock()
		return &HubEvent{Kind: EvPeers, Peers: peers}

	case "peer-join":
		s.mu.Lock()
		if live {
			s.peers = append(s.peers, PeerEntry{Name: name, Connected: time.Now()})
		}
		snapshot := append([]PeerEntry{}, s.peers...)
		s.mu.Unlock()
		return &HubEvent{Kind: EvJoin, Name: name, Peers: snapshot}

	case "peer-leave":
		s.mu.Lock()
		if live {
			s.peers = removePeer(s.peers, name)
		}
		snapshot := append([]PeerEntry{}, s.peers...)
		s.mu.Unlock()
		return &HubEvent{Kind: EvLeave, Name: name, Peers: snapshot}

	case "kick":
		s.mu.Lock()
		if live {
			s.peers = removePeer(s.peers, name)
		}
		snapshot := append([]PeerEntry{}, s.peers...)
		s.mu.Unlock()
		return &HubEvent{Kind: EvKick, Name: name, Text: reason, Peers: snapshot}

	case "chat":
		return &HubEvent{Kind: EvChat, Name: name, Text: text}

	case "chat-direct":
		return &HubEvent{Kind: EvDirect, Name: name, Target: to, Text: text}

	case "say":
		return &HubEvent{Kind: EvSay, Text: text}

	case "motd-changed":
		return &HubEvent{Kind: EvMotd, Text: text}
	}
	return nil
}

func (s *remoteSource) parsePeers(m map[string]any) []PeerEntry {
	raw, _ := m["peers"].([]any)
	result := make([]PeerEntry, 0, len(raw))
	for _, item := range raw {
		pm, _ := item.(map[string]any)
		name, _ := pm["name"].(string)
		ip, _ := pm["ip"].(string)
		connected := time.Now()
		if cs, ok := pm["connected"].(string); ok {
			if t, err := time.Parse(time.RFC3339, cs); err == nil {
				connected = t
			}
		}
		result = append(result, PeerEntry{Name: name, RemoteIP: ip, Connected: connected})
	}
	return result
}

func removePeer(peers []PeerEntry, name string) []PeerEntry {
	out := peers[:0]
	for _, p := range peers {
		if p.Name != name {
			out = append(out, p)
		}
	}
	return out
}

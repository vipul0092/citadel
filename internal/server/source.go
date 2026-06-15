package server

import (
	"sync"
	"time"

	"github.com/vipul0092/citadel/internal/control"
	ctrlclient "github.com/vipul0092/citadel/internal/control/client"
)


// HubEventSource is the abstraction the server TUI uses to receive events and dispatch
// admin commands. Two implementations:
//   - inProcessSource: combines *Hub (commands) with fanOutEmitter.TUIEvents (events).
//   - remoteSource: backed by a control-plane connection for dashboard drill-in.
type HubEventSource interface {
	Events() <-chan HubEvent
	Kick(name, reason string) bool
	Say(text string)
	SetMotd(text string)
	Peers() []PeerEntry
}

// --- in-process source ---

// inProcessSource combines a *Hub (for command dispatch) with the TUI-side channel
// from fanOutEmitter (for event receipt). It is the adapter used when TUI and Hub
// live in the same process.
type inProcessSource struct {
	hub      *Hub
	eventsCh <-chan HubEvent
}

// NewInProcessSource wraps hub and the TUI events channel into a HubEventSource.
func NewInProcessSource(hub *Hub, ch <-chan HubEvent) HubEventSource {
	return &inProcessSource{hub: hub, eventsCh: ch}
}

func (s *inProcessSource) Events() <-chan HubEvent            { return s.eventsCh }
func (s *inProcessSource) Kick(name, reason string) bool      { return s.hub.Kick(name, reason) }
func (s *inProcessSource) Say(text string)                    { s.hub.Say(text) }
func (s *inProcessSource) SetMotd(text string)                { s.hub.SetMotd(text) }
func (s *inProcessSource) Peers() []PeerEntry                 { return s.hub.Peers() }

// --- remote source ---

type remoteSource struct {
	conn     *ctrlclient.Subscriber
	eventsCh chan HubEvent
	mu       sync.Mutex
	peers    []PeerEntry
}

// NewRemoteSource dials the server's control socket, subscribes at full level,
// and returns a HubEventSource that translates control-plane events to HubEvents.
func NewRemoteSource(sockPath, name, addr string) (HubEventSource, error) {
	sub, err := ctrlclient.DialAndSubscribe(sockPath, "full", 0)
	if err != nil {
		return nil, err
	}

	s := &remoteSource{
		conn:     sub,
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
	for ev := range s.conn.Events() {
		switch ev.Kind {
		case control.KindLive:
			_ = s.conn.Send(map[string]any{"op": "list-peers"})
			peersSynced = true
		case control.KindPeers:
			entries := toPeerEntries(ev.Peers)
			s.mu.Lock()
			s.peers = entries
			s.mu.Unlock()
			s.eventsCh <- HubEvent{Kind: EvPeers, Peers: append([]PeerEntry{}, entries...)}
		case control.KindPeerJoin:
			s.mu.Lock()
			if peersSynced {
				s.peers = append(s.peers, PeerEntry{Name: ev.Name, Connected: time.Now()})
			}
			snapshot := append([]PeerEntry{}, s.peers...)
			s.mu.Unlock()
			s.eventsCh <- HubEvent{Kind: EvJoin, Name: ev.Name, Peers: snapshot}
		case control.KindPeerLeave:
			s.mu.Lock()
			if peersSynced {
				s.peers = removePeer(s.peers, ev.Name)
			}
			snapshot := append([]PeerEntry{}, s.peers...)
			s.mu.Unlock()
			s.eventsCh <- HubEvent{Kind: EvLeave, Name: ev.Name, Peers: snapshot}
		case control.KindKick:
			s.mu.Lock()
			if peersSynced {
				s.peers = removePeer(s.peers, ev.Name)
			}
			snapshot := append([]PeerEntry{}, s.peers...)
			s.mu.Unlock()
			s.eventsCh <- HubEvent{Kind: EvKick, Name: ev.Name, Text: ev.Reason, Peers: snapshot}
		case control.KindChat:
			s.eventsCh <- HubEvent{Kind: EvChat, Name: ev.Name, Text: ev.Text}
		case control.KindChatDirect:
			s.eventsCh <- HubEvent{Kind: EvDirect, Name: ev.Name, Target: ev.To, Text: ev.Text}
		case control.KindSay:
			s.eventsCh <- HubEvent{Kind: EvSay, Text: ev.Text}
		case control.KindMotd:
			s.eventsCh <- HubEvent{Kind: EvMotd, Text: ev.Text}
		}
	}
	close(s.eventsCh)
}

func toPeerEntries(ps []control.PeerInfo) []PeerEntry {
	result := make([]PeerEntry, 0, len(ps))
	for _, p := range ps {
		connected := time.Now()
		if t, err := time.Parse(time.RFC3339, p.Connected); err == nil {
			connected = t
		}
		result = append(result, PeerEntry{Name: p.Name, RemoteIP: p.IP, Connected: connected})
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

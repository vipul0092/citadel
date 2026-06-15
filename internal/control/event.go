package control

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	ringCap      = 200
	subQueueSize = 64 // outbound queue depth; mirrors internal/server/client.go:43-52
)

// Level is the subscription granularity.
type Level int

const (
	LevelSummary Level = iota
	LevelFull
)

// ringEntry is one entry in the ring buffer (internal to this package).
type ringEntry struct {
	Seq  uint64
	At   time.Time
	Kind string
	Data json.RawMessage
}

// levelIncludes reports whether kind should be delivered at level l.
func levelIncludes(l Level, kind string) bool {
	switch kind {
	case "status", "peers", "peer-join", "peer-leave":
		return true // summary and full
	case "chat", "chat-direct", "say", "motd-changed", "kick", "system":
		return l == LevelFull
	}
	return false
}

// EventKind is the discriminator for a decoded control-plane ringEntry frame.
type EventKind string

const (
	KindPeerJoin   EventKind = "peer-join"
	KindPeerLeave  EventKind = "peer-leave"
	KindKick       EventKind = "kick"
	KindChat       EventKind = "chat"
	KindChatDirect EventKind = "chat-direct"
	KindSay        EventKind = "say"
	KindMotd       EventKind = "motd-changed"
	KindPeers      EventKind = "peers"
	KindLive       EventKind = "live"
	KindBye        EventKind = "bye"
	KindSystem     EventKind = "system"
	KindStatus     EventKind = "status"
	KindGame       EventKind = "game"
)

// Event is a decoded control-plane ringEntry frame received by a subscriber.
//
// Which fields are populated depends on Kind:
//
//	KindPeerJoin, KindPeerLeave          → Name
//	KindKick                             → Name, Reason
//	KindChat                             → Name, Text
//	KindChatDirect                       → Name, To, Text
//	KindSay, KindMotd                    → Text
//	KindPeers                            → Peers (PeerInfo.Connected is RFC3339 string)
//	KindBye                              → Reason
//	KindGame                             → Name (sender), GameKind, To, Data (opaque)
//	KindLive, KindStatus, KindSystem     → no payload fields
type Event struct {
	Kind     EventKind
	Name     string
	Text     string
	To       string
	Reason   string
	Peers    []PeerInfo
	GameKind string
	Data     json.RawMessage
}

// Decode parses a raw control-plane wire frame into a typed Event.
// Unknown ev values return a zero Event with Kind set and no error.
func Decode(frame []byte) (Event, error) {
	var raw struct {
		Ev     string          `json:"ev"`
		Name   string          `json:"name"`
		Text   string          `json:"text"`
		To     string          `json:"to"`
		From   string          `json:"from"`
		Reason string          `json:"reason"`
		Kind   string          `json:"kind"`
		Data   json.RawMessage `json:"data"`
		Peers  json.RawMessage `json:"peers"`
	}
	if err := json.Unmarshal(frame, &raw); err != nil {
		return Event{}, fmt.Errorf("decode control ringEntry: %w", err)
	}

	ev := Event{Kind: EventKind(raw.Ev)}
	switch ev.Kind {
	case KindPeerJoin, KindPeerLeave:
		ev.Name = raw.Name
	case KindKick:
		ev.Name = raw.Name
		ev.Reason = raw.Reason
	case KindChat:
		ev.Name = raw.Name
		ev.Text = raw.Text
	case KindChatDirect:
		ev.Name = raw.Name
		ev.To = raw.To
		ev.Text = raw.Text
	case KindSay:
		ev.Text = raw.Text
	case KindMotd:
		ev.Text = raw.Text
	case KindPeers:
		peers, err := decodePeerInfos(raw.Peers)
		if err != nil {
			return Event{}, fmt.Errorf("decode peers: %w", err)
		}
		ev.Peers = peers
	case KindBye:
		ev.Reason = raw.Reason
	case KindGame:
		ev.Name = raw.From
		ev.GameKind = raw.Kind
		ev.To = raw.To
		ev.Data = raw.Data
	}
	return ev, nil
}

func decodePeerInfos(raw json.RawMessage) ([]PeerInfo, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var peers []PeerInfo
	if err := json.Unmarshal(raw, &peers); err != nil {
		return nil, err
	}
	return peers, nil
}

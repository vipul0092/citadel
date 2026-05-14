package control

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/vipul0092/citadel/internal/proto"
)

// Hub is the control-plane fan-out hub.
// It owns the ring buffer and all subscriber state.
type Hub struct {
	ring *ring
	mu   sync.Mutex
	subs map[*sub]struct{}
}

// NewHub creates a Hub ready to use.
func NewHub() *Hub {
	return &Hub{
		ring: newRing(),
		subs: make(map[*sub]struct{}),
	}
}

// Subscribe creates a subscription at the given level and registers it in the fan-out set.
// The historic snapshot (ring contents since `since`) is written to the sub's replayCh
// outside the lock; the attacher goroutine drains it before entering the live stream.
func (h *Hub) Subscribe(level Level, since uint64, wantGame bool) *sub {
	s := newSub(level, wantGame)

	h.mu.Lock()
	gap, events := h.ring.snapshot(since, level)
	h.subs[s] = struct{}{} // add before unlock so no Emit is missed
	h.mu.Unlock()

	s.replayCh <- replayPkt{gap: gap, events: events} // non-blocking: buffered-1, sub is new
	return s
}

// Unsubscribe removes s from the fan-out set and closes it.
func (h *Hub) Unsubscribe(s *sub) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
	s.close()
}

// Emit appends an event to the ring and delivers it to all matching subscribers.
// Game payloads must use EmitGame instead — they bypass the ring.
func (h *Hub) Emit(kind string, data json.RawMessage) {
	h.mu.Lock()
	ev := h.ring.append(kind, data)
	// snapshot subscriber set while holding the lock
	subs := make([]*sub, 0, len(h.subs))
	for s := range h.subs {
		subs = append(subs, s)
	}
	h.mu.Unlock()

	frame, err := buildEventFrame(ev)
	if err != nil {
		slog.Error("control hub: encode event", "kind", kind, "err", err)
		return
	}

	var toDrop []*sub
	for _, s := range subs {
		if !levelIncludes(s.level, kind) {
			continue
		}
		if !s.trySendLive(frame) {
			toDrop = append(toDrop, s)
		}
	}
	for _, s := range toDrop {
		slog.Warn("control hub: slow attacher dropped")
		h.dropSub(s)
	}
}

// EmitGame delivers a game payload to all subscribers that opted into game events.
// Game payloads are never added to the ring (live-stream only, no replay).
func (h *Hub) EmitGame(from, kind, to string, data json.RawMessage) {
	frame, err := buildGameFrame(from, kind, to, data)
	if err != nil {
		slog.Error("control hub: encode game event", "err", err)
		return
	}

	h.mu.Lock()
	subs := make([]*sub, 0, len(h.subs))
	for s := range h.subs {
		if s.wantGame {
			subs = append(subs, s)
		}
	}
	h.mu.Unlock()

	var toDrop []*sub
	for _, s := range subs {
		if !s.trySendGame(frame) {
			toDrop = append(toDrop, s)
		}
	}
	for _, s := range toDrop {
		slog.Warn("control hub: slow game attacher dropped")
		h.dropSub(s)
	}
}

// SetGameSub toggles the game subscription flag for s. Called by the attacher goroutine
// when it processes subscribe-game / unsubscribe-game ops.
func (h *Hub) SetGameSub(s *sub, want bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[s]; !ok {
		return
	}
	s.wantGame = want
	if want && s.gameCh == nil {
		s.gameCh = make(chan []byte, subQueueSize)
	}
}

// ForwardEnvelope translates a wire-protocol envelope received by a client Conn
// into a control-plane event and emits it on the hub (or EmitGame for game payloads).
func (h *Hub) ForwardEnvelope(env *proto.Envelope) {
	switch env.Type {
	case proto.TypeGame:
		var p proto.GamePayload
		if err := proto.UnmarshalPayload(env, &p); err != nil {
			return
		}
		data, _ := json.Marshal(p.Data)
		h.EmitGame(env.From, p.Kind, env.To, data)

	case proto.TypeSystem:
		var p proto.SystemPayload
		if err := proto.UnmarshalPayload(env, &p); err != nil {
			return
		}
		h.forwardSystem(p)

	case proto.TypeChat:
		var p proto.ChatPayload
		if err := proto.UnmarshalPayload(env, &p); err != nil {
			return
		}
		if env.To != "" {
			data, _ := json.Marshal(struct {
				Name string `json:"name"`
				To   string `json:"to"`
				Text string `json:"text"`
			}{Name: env.From, To: env.To, Text: p.Text})
			h.Emit("chat-direct", data)
		} else {
			data, _ := json.Marshal(struct {
				Name string `json:"name"`
				Text string `json:"text"`
			}{Name: env.From, Text: p.Text})
			h.Emit("chat", data)
		}

	case proto.TypeKick:
		var p proto.KickPayload
		if err := proto.UnmarshalPayload(env, &p); err != nil {
			return
		}
		data, _ := json.Marshal(struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		}{Name: env.To, Reason: p.Reason})
		h.Emit("kick", data)

	// TypeHello, TypeWelcome, TypeReject, TypePing, TypePong, TypeLeave: not forwarded
	}
}

func (h *Hub) forwardSystem(p proto.SystemPayload) {
	switch p.Event {
	case proto.EvJoin:
		data, _ := json.Marshal(struct {
			Name string `json:"name"`
		}{Name: p.Name})
		h.Emit("peer-join", data)

	case proto.EvLeave:
		data, _ := json.Marshal(struct {
			Name string `json:"name"`
		}{Name: p.Name})
		h.Emit("peer-leave", data)

	case proto.EvKick:
		data, _ := json.Marshal(struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		}{Name: p.Name, Reason: p.Message})
		h.Emit("kick", data)

	case proto.EvMotd:
		data, _ := json.Marshal(struct {
			Text string `json:"text"`
		}{Text: p.Message})
		h.Emit("motd-changed", data)

	case proto.EvTooLong:
		data, _ := json.Marshal(struct {
			Event string `json:"event"`
			Name  string `json:"name,omitempty"`
		}{Event: string(p.Event), Name: p.Name})
		h.Emit("system", data)
	}
}

// SendBye delivers a bye frame to all current subscribers (non-blocking; does not block
// on slow consumers). Called by Plane.Close before tearing down the listener.
func (h *Hub) SendBye(reason string) {
	frame, err := buildSimpleFrame(struct {
		Ev     string `json:"ev"`
		Reason string `json:"reason"`
	}{Ev: "bye", Reason: reason})
	if err != nil {
		return
	}

	h.mu.Lock()
	subs := make([]*sub, 0, len(h.subs))
	for s := range h.subs {
		subs = append(subs, s)
	}
	h.mu.Unlock()

	for _, s := range subs {
		s.trySendLive(frame) // best-effort; process is shutting down
	}
}

func (h *Hub) dropSub(s *sub) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
	s.close()
}

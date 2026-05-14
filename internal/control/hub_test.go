package control

import (
	"encoding/json"
	"testing"
)

func TestHubSlowDrop(t *testing.T) {
	h := NewHub()
	s := h.Subscribe(LevelFull, 0, false)

	// Drain the replay packet so liveCh is available.
	<-s.replayCh

	// Fill liveCh to capacity.
	frame, _ := json.Marshal(struct{ X int }{1})
	for i := 0; i < subQueueSize; i++ {
		s.liveCh <- frame
	}

	// One more Emit should trigger slow-drop and remove s from hub.
	h.Emit("chat", rawJSON(`{"name":"a","text":"hi"}`))

	// Verify s is no longer in the subs map.
	h.mu.Lock()
	_, stillPresent := h.subs[s]
	h.mu.Unlock()
	if stillPresent {
		t.Error("slow attacher should have been removed from hub.subs")
	}

	// s.done should be closed.
	select {
	case <-s.done:
	default:
		t.Error("s.done should be closed after drop")
	}
}

func TestHubEmitReplayOrder(t *testing.T) {
	h := NewHub()

	// First subscriber at since=0.
	s1 := h.Subscribe(LevelFull, 0, false)
	<-s1.replayCh // no events yet

	// Emit 3 events.
	h.Emit("peer-join", rawJSON(`{"name":"a"}`))
	h.Emit("chat", rawJSON(`{"name":"a","text":"hi"}`))
	h.Emit("peer-leave", rawJSON(`{"name":"a"}`))

	// Second subscriber at since=1 — should replay only seq 2 and 3.
	s2 := h.Subscribe(LevelFull, 1, false)
	pkt := <-s2.replayCh
	if pkt.gap != nil {
		t.Errorf("unexpected gap: %+v", pkt.gap)
	}
	if len(pkt.events) != 2 {
		t.Errorf("expected 2 replay events (seq 2-3), got %d", len(pkt.events))
	}
	if pkt.events[0].Seq != 2 || pkt.events[1].Seq != 3 {
		t.Errorf("wrong seqs: %d, %d", pkt.events[0].Seq, pkt.events[1].Seq)
	}

	h.Unsubscribe(s1)
	h.Unsubscribe(s2)
}

func TestHubGameNotBuffered(t *testing.T) {
	h := NewHub()
	s := h.Subscribe(LevelFull, 0, true)
	<-s.replayCh

	// EmitGame should NOT appear in ring.
	h.EmitGame("alice", "move", "", rawJSON(`{"x":1}`))

	h.mu.Lock()
	count := h.ring.count
	h.mu.Unlock()
	if count != 0 {
		t.Errorf("game event should not be in ring, but ring.count=%d", count)
	}

	// Game frame should arrive on gameCh.
	select {
	case <-s.gameCh:
	default:
		t.Error("expected game frame on gameCh")
	}

	h.Unsubscribe(s)
}

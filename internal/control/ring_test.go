package control

import (
	"encoding/json"
	"testing"
)

func rawJSON(s string) json.RawMessage { return json.RawMessage(s) }

func TestRingSeqMonotonicity(t *testing.T) {
	t.Helper()
	r := newRing()
	r.mu.Lock()
	defer r.mu.Unlock()

	var prev uint64
	for i := 0; i < ringCap*2; i++ {
		ev := r.append("peer-join", rawJSON(`{"name":"x"}`))
		if ev.Seq <= prev {
			t.Fatalf("seq not monotonic: got %d after %d", ev.Seq, prev)
		}
		prev = ev.Seq
	}
}

func TestRingReplay_SeqFilter(t *testing.T) {
	r := newRing()
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := 0; i < 5; i++ {
		r.append("peer-join", rawJSON(`{"name":"x"}`))
	}

	_, events := r.snapshot(2, LevelSummary)
	if len(events) != 3 {
		t.Fatalf("expected 3 events (seq 3-5), got %d", len(events))
	}
	for i, ev := range events {
		want := uint64(3 + i)
		if ev.Seq != want {
			t.Errorf("event[%d]: want seq %d, got %d", i, want, ev.Seq)
		}
	}
}

func TestRingLevelFilter(t *testing.T) {
	r := newRing()
	r.mu.Lock()
	defer r.mu.Unlock()

	r.append("peer-join", rawJSON(`{}`))  // summary
	r.append("chat", rawJSON(`{}`))       // full only
	r.append("peer-leave", rawJSON(`{}`)) // summary

	_, summaryEvs := r.snapshot(0, LevelSummary)
	_, fullEvs := r.snapshot(0, LevelFull)

	if len(summaryEvs) != 2 {
		t.Errorf("summary: want 2 events, got %d", len(summaryEvs))
	}
	if len(fullEvs) != 3 {
		t.Errorf("full: want 3 events, got %d", len(fullEvs))
	}
}

func TestRingGap(t *testing.T) {
	r := newRing()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Fill more than ringCap entries to push oldest seq past 1.
	for i := 0; i < ringCap+10; i++ {
		r.append("peer-join", rawJSON(`{}`))
	}

	oldest := r.oldest()
	if oldest < 2 {
		t.Fatalf("expected oldest > 1 after overflow, got %d", oldest)
	}

	// since=1 is older than oldest-1, so we expect a gap.
	gap, _ := r.snapshot(1, LevelSummary)
	if gap == nil {
		t.Fatal("expected gap, got nil")
	}
	if gap.MissingFrom != 2 {
		t.Errorf("gap.MissingFrom: want 2, got %d", gap.MissingFrom)
	}
	if gap.MissingTo != oldest-1 {
		t.Errorf("gap.MissingTo: want %d, got %d", oldest-1, gap.MissingTo)
	}
}

func TestRingNoGap_WhenSinceIsRecent(t *testing.T) {
	r := newRing()
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := 0; i < 5; i++ {
		r.append("peer-join", rawJSON(`{}`))
	}

	// since=3 is within the ring, no gap expected.
	gap, events := r.snapshot(3, LevelSummary)
	if gap != nil {
		t.Errorf("expected no gap, got %+v", gap)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events (seq 4-5), got %d", len(events))
	}
}

func TestRingEmpty(t *testing.T) {
	r := newRing()
	r.mu.Lock()
	defer r.mu.Unlock()

	gap, events := r.snapshot(0, LevelFull)
	if gap != nil || len(events) != 0 {
		t.Errorf("empty ring: expected nil gap and no events, got gap=%v events=%d", gap, len(events))
	}
}

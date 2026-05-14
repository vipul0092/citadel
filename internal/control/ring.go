package control

import (
	"encoding/json"
	"sync"
	"time"
)

// gapInfo describes a hole in the sequence space that a replay cannot fill.
type gapInfo struct {
	MissingFrom uint64
	MissingTo   uint64
}

// ring is a fixed-capacity circular buffer of control-plane Events.
// All methods that read or mutate state require the caller to hold mu.
type ring struct {
	mu      sync.Mutex
	entries [ringCap]Event
	head    int    // next write index (mod ringCap)
	count   int    // number of valid entries (0..ringCap)
	nextSeq uint64 // seq to assign on the next append (starts at 1)
}

func newRing() *ring {
	return &ring{nextSeq: 1}
}

// append stores a new event in the ring and returns it with seq and At filled in.
// Caller must hold r.mu.
func (r *ring) append(kind string, data json.RawMessage) Event {
	ev := Event{
		Seq:  r.nextSeq,
		At:   time.Now().UTC(),
		Kind: kind,
		Data: data,
	}
	r.nextSeq++
	r.entries[r.head] = ev
	r.head = (r.head + 1) % ringCap
	if r.count < ringCap {
		r.count++
	}
	return ev
}

// oldest returns the seq of the oldest entry, or 0 if the ring is empty.
// Caller must hold r.mu.
func (r *ring) oldest() uint64 {
	if r.count == 0 {
		return 0
	}
	idx := (r.head - r.count + ringCap) % ringCap
	return r.entries[idx].Seq
}

// snapshot returns events with seq > since that match level, and a gap descriptor
// when since is too old for the ring to cover.
// Caller must hold r.mu.
func (r *ring) snapshot(since uint64, level Level) (*gapInfo, []Event) {
	if r.count == 0 {
		return nil, nil
	}

	var gap *gapInfo
	oldest := r.oldest()
	// since < oldest-1 means there are entries between since and the ring's start
	// that we cannot replay. oldest-1 is safe even when oldest==1 (uint underflow)
	// because the condition also requires since > 0.
	if since > 0 && oldest > 0 && since < oldest-1 {
		gap = &gapInfo{MissingFrom: since + 1, MissingTo: oldest - 1}
	}

	var events []Event
	for i := 0; i < r.count; i++ {
		idx := (r.head - r.count + i + ringCap) % ringCap
		ev := r.entries[idx]
		if ev.Seq > since && levelIncludes(level, ev.Kind) {
			events = append(events, ev)
		}
	}
	return gap, events
}

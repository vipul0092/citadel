package control

import (
	"encoding/json"
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

// Event is one entry in the ring buffer.
type Event struct {
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

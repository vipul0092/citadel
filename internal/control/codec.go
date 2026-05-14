package control

import (
	"encoding/json"
	"fmt"
	"time"
)

// buildEventFrame encodes a ring-buffer Event into the wire JSON shape:
//
//	{"ev":"<kind>","seq":<n>,"at":"<rfc3339>",<kind-specific fields>}
//
// The kind-specific payload is spread into the top-level object.
func buildEventFrame(ev Event) ([]byte, error) {
	type header struct {
		Ev  string `json:"ev"`
		Seq uint64 `json:"seq"`
		At  string `json:"at"`
	}
	hdr, err := json.Marshal(header{
		Ev:  ev.Kind,
		Seq: ev.Seq,
		At:  ev.At.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal event header: %w", err)
	}
	return mergeJSON(hdr, ev.Data), nil
}

// buildGameFrame encodes a game payload event (no seq/at, never buffered).
func buildGameFrame(from, kind, to string, data json.RawMessage) ([]byte, error) {
	type gameEnv struct {
		Ev   string          `json:"ev"`
		From string          `json:"from"`
		Kind string          `json:"kind"`
		To   string          `json:"to,omitempty"`
		Data json.RawMessage `json:"data,omitempty"`
	}
	return json.Marshal(gameEnv{Ev: "game", From: from, Kind: kind, To: to, Data: data})
}

// buildSimpleFrame marshals v to JSON.
func buildSimpleFrame(v any) ([]byte, error) {
	return json.Marshal(v)
}

// peekOp extracts the "op" discriminator from a raw JSON object.
func peekOp(raw []byte) (string, error) {
	var m struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("decode op: %w", err)
	}
	if m.Op == "" {
		return "", fmt.Errorf("missing op field")
	}
	return m.Op, nil
}

// Op payload structs (attacher → citadel).

type subscribeOp struct {
	Level string `json:"level"`
	Since uint64 `json:"since"`
}

type setLevelOp struct {
	Level string `json:"level"`
	Since uint64 `json:"since"`
}

// parseLevel converts a level string to a Level constant, defaulting to LevelSummary.
func parseLevel(s string) Level {
	if s == "full" {
		return LevelFull
	}
	return LevelSummary
}

// mergeJSON merges two JSON objects by stripping the trailing "}" from a and
// prepending with the opening "{" of b, inserting a comma separator.
// Both a and b must be valid JSON objects (not just "{}").
func mergeJSON(a, b []byte) []byte {
	if len(b) <= 2 { // "{}" or empty — nothing to merge
		return a
	}
	// a ends with "}", b starts with "{"
	// result: a[:-1] + "," + b[1:]
	out := make([]byte, 0, len(a)+len(b))
	out = append(out, a[:len(a)-1]...)
	out = append(out, ',')
	out = append(out, b[1:]...)
	return out
}

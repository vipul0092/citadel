package proto

import (
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
)

// Message type constants used in Envelope.Type.
const (
	TypeHello   = "hello"
	TypeWelcome = "welcome"
	TypeReject  = "reject"
	TypeChat    = "chat"
	TypeKick    = "kick"
	TypeLeave   = "leave"
	TypeSystem  = "system"
	TypePing    = "ping"
	TypePong    = "pong"
	TypeGame    = "game"
)

// Envelope wraps every message on the wire.
// From is server-derived on receive; clients must not trust Envelope.From from peers.
type Envelope struct {
	Type    string          `json:"type"`
	From    string          `json:"from,omitempty"`
	To      string          `json:"to,omitempty"`
	Seq     uint64          `json:"seq"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

var globalSeq atomic.Uint64

// Encode marshals payload, wraps it in an Envelope, and writes it as a frame to w.
func Encode(w io.Writer, msgType, from, to string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}
	env := Envelope{
		Type:    msgType,
		From:    from,
		To:      to,
		Seq:     globalSeq.Add(1),
		Payload: raw,
	}
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshaling envelope: %w", err)
	}
	return WriteFrame(w, data)
}

// Decode reads one frame from r and returns the parsed Envelope.
func Decode(r io.Reader) (*Envelope, error) {
	data, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("unmarshaling envelope: %w", err)
	}
	return &env, nil
}

// UnmarshalPayload decodes env.Payload into target.
func UnmarshalPayload(env *Envelope, target any) error {
	if err := json.Unmarshal(env.Payload, target); err != nil {
		return fmt.Errorf("unmarshaling %s payload: %w", env.Type, err)
	}
	return nil
}

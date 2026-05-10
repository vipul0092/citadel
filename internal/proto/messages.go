package proto

import "encoding/json"

// HelloPayload is sent by the client immediately after dialing.
type HelloPayload struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// WelcomePayload is sent by the server on successful handshake.
type WelcomePayload struct {
	ServerName string   `json:"server_name"`
	Motd       string   `json:"motd,omitempty"`
	Peers      []string `json:"peers"`
}

// RejectPayload is sent by the server when the client is denied; server closes after sending.
type RejectPayload struct {
	Reason string `json:"reason"`
}

// ChatPayload is a broadcast chat or /msg direct message.
// To is empty for broadcast; the hub routes to only that peer when non-empty.
type ChatPayload struct {
	Text string `json:"text"`
	To   string `json:"to,omitempty"`
}

// KickPayload notifies the kicked client; server closes after sending.
type KickPayload struct {
	Reason string `json:"reason"`
}

// LeavePayload is sent by the client before closing.
type LeavePayload struct{}

// SystemEvent is the discriminator inside SystemPayload.
type SystemEvent string

const (
	EvJoin    SystemEvent = "join"
	EvLeave   SystemEvent = "leave"
	EvKick    SystemEvent = "kick"
	EvMotd    SystemEvent = "motd"
	EvTooLong SystemEvent = "too_long"
)

// SystemPayload carries server-to-all-clients notifications.
type SystemPayload struct {
	Event   SystemEvent `json:"event"`
	Name    string      `json:"name,omitempty"`
	Message string      `json:"message,omitempty"`
}

// PingPayload is the client heartbeat.
type PingPayload struct{}

// PongPayload is the server heartbeat reply.
type PongPayload struct{}

// GamePayload wraps an opaque game message. The server relays it without inspecting Kind/Data.
type GamePayload struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data,omitempty"`
}

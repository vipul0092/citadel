package control

import "encoding/json"

// PeerInfo describes a connected peer for list-peers responses.
type PeerInfo struct {
	Name      string `json:"name"`
	IP        string `json:"ip,omitempty"`
	Connected string `json:"connected,omitempty"`
}

// CommonActions is implemented by both server and client roles.
type CommonActions interface {
	ListPeers() []PeerInfo
}

// ServerActions is implemented by the server role only.
type ServerActions interface {
	KickPeer(name, reason string) (bool, error)
	SayAll(text string) error
	SetMotd(text string) error
}

// ClientActions is implemented by the client role only.
type ClientActions interface {
	SendChat(text, to string) error
	SendGame(kind, to string, data json.RawMessage) error
}

// RoleActions bundles the action interfaces for one citadel process.
// Common is always non-nil. Exactly one of Server or Client is non-nil,
// determined by the role of the process.
type RoleActions struct {
	Common CommonActions
	Server ServerActions // non-nil for server role; nil for client role
	Client ClientActions // non-nil for client role; nil for server role
}

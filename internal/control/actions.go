package control

import (
	"encoding/json"
	"errors"
)

// ErrNotSupported is returned by ActionsProvider implementations for ops that do not
// apply to their role (e.g. kick on a client socket).
var ErrNotSupported = errors.New("op not supported for this role")

// PeerInfo describes a connected peer for list-peers responses.
type PeerInfo struct {
	Name      string `json:"name"`
	IP        string `json:"ip,omitempty"`
	Connected string `json:"connected,omitempty"`
}

// ActionsProvider is implemented by server and client code to expose ops that the
// control-plane listener can trigger on behalf of an attacher.
//
// Methods that do not apply to the current role must return ErrNotSupported.
type ActionsProvider interface {
	// ListPeers returns the currently-connected peers (both roles).
	ListPeers() []PeerInfo

	// Server-only ops.
	KickPeer(name, reason string) (bool, error)
	SayAll(text string) error
	SetMotd(text string) error

	// Client-only ops.
	SendChat(text, to string) error
	SendGame(kind, to string, data json.RawMessage) error
}

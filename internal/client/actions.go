package client

import (
	"encoding/json"

	"github.com/vipul0092/citadel/internal/control"
	"github.com/vipul0092/citadel/internal/proto"
)

type connActions struct{ conn *Conn }

// NewActionsProvider returns an actions implementation backed by the client Conn.
func NewActionsProvider(c *Conn) *connActions { return &connActions{conn: c} }

func (a *connActions) ListPeers() []control.PeerInfo {
	out := make([]control.PeerInfo, len(a.conn.peers))
	for i, name := range a.conn.peers {
		out[i] = control.PeerInfo{Name: name}
	}
	return out
}

func (a *connActions) SendChat(text, to string) error {
	return a.conn.Send(proto.TypeChat, to, proto.ChatPayload{Text: text, To: to})
}

func (a *connActions) SendGame(kind, to string, data json.RawMessage) error {
	return a.conn.Send(proto.TypeGame, to, proto.GamePayload{Kind: kind, Data: data})
}

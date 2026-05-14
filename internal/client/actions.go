package client

import (
	"encoding/json"

	"github.com/vipul0092/citadel/internal/control"
	"github.com/vipul0092/citadel/internal/proto"
)

type connActions struct{ conn *Conn }

// NewActionsProvider returns a control.ActionsProvider backed by the client Conn.
func NewActionsProvider(c *Conn) control.ActionsProvider { return &connActions{conn: c} }

func (a *connActions) ListPeers() []control.PeerInfo {
	out := make([]control.PeerInfo, len(a.conn.Peers))
	for i, name := range a.conn.Peers {
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

func (a *connActions) KickPeer(_, _ string) (bool, error) {
	return false, control.ErrNotSupported
}

func (a *connActions) SayAll(_ string) error { return control.ErrNotSupported }

func (a *connActions) SetMotd(_ string) error { return control.ErrNotSupported }

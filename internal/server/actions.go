package server

import (
	"encoding/json"

	"github.com/vipul0092/citadel/internal/control"
)

type serverActions struct{ hub *Hub }

// NewActionsProvider returns a control.ActionsProvider backed by the server Hub.
func NewActionsProvider(h *Hub) control.ActionsProvider { return &serverActions{hub: h} }

func (a *serverActions) ListPeers() []control.PeerInfo {
	peers := a.hub.Peers()
	out := make([]control.PeerInfo, len(peers))
	for i, p := range peers {
		out[i] = control.PeerInfo{
			Name:      p.Name,
			IP:        p.RemoteIP,
			Connected: p.Connected.UTC().String(),
		}
	}
	return out
}

func (a *serverActions) KickPeer(name, reason string) (bool, error) {
	return a.hub.Kick(name, reason), nil
}

func (a *serverActions) SayAll(text string) error {
	a.hub.Say(text)
	return nil
}

func (a *serverActions) SetMotd(text string) error {
	a.hub.SetMotd(text)
	return nil
}

func (a *serverActions) SendChat(_, _ string) error {
	return control.ErrNotSupported
}

func (a *serverActions) SendGame(_, _ string, _ json.RawMessage) error {
	return control.ErrNotSupported
}

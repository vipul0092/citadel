package client

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/vipul0092/citadel/internal/control"
	ctrlclient "github.com/vipul0092/citadel/internal/control/client"
	"github.com/vipul0092/citadel/internal/proto"
)

// ConnController is the abstraction the client TUI uses for all post-handshake
// send/receive operations. Two implementations:
//   - inProcessController: wraps *Conn for the normal in-process TUI.
//   - remoteConnController: backed by a control-plane connection for dashboard drill-in.
type ConnController interface {
	Recv() (*proto.Envelope, error)
	Send(msgType, to string, payload any) error
	Name() string
	ServerName() string
	Motd() string
	Peers() []string
	Close()
}

// --- remote controller ---

type remoteConnController struct {
	conn       *ctrlclient.Subscriber
	recvCh     chan *proto.Envelope
	myName     string
	serverName string
	mu         sync.Mutex
	peers      []string
}

// NewRemoteController dials the client's control socket and returns a ConnController
// that the dashboard drill-in TUI can use to send/receive as if it were the real Conn.
func NewRemoteController(sockPath, myName, displayServerName string) (ConnController, error) {
	sub, err := ctrlclient.DialAndSubscribe(sockPath, "full", 0)
	if err != nil {
		return nil, err
	}
	_ = sub.Send(map[string]any{"op": "list-peers"})

	c := &remoteConnController{
		conn:       sub,
		recvCh:     make(chan *proto.Envelope, 64),
		myName:     myName,
		serverName: displayServerName,
	}
	go c.translate()
	return c, nil
}

func (c *remoteConnController) Name() string       { return c.myName }
func (c *remoteConnController) ServerName() string { return c.serverName }
func (c *remoteConnController) Motd() string       { return "" }
func (c *remoteConnController) Close()             { c.conn.Close() }

func (c *remoteConnController) Peers() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.peers...)
}

func (c *remoteConnController) Recv() (*proto.Envelope, error) {
	env, ok := <-c.recvCh
	if !ok {
		return nil, fmt.Errorf("connection closed")
	}
	return env, nil
}

func (c *remoteConnController) Send(msgType, to string, payload any) error {
	switch msgType {
	case proto.TypeChat:
		p, _ := payload.(proto.ChatPayload)
		return c.conn.Send(map[string]any{"op": "send-chat", "text": p.Text, "to": to})
	case proto.TypeGame:
		p, _ := payload.(proto.GamePayload)
		return c.conn.Send(map[string]any{"op": "send-game", "kind": p.Kind, "to": to, "data": p.Data})
	}
	return nil
}

func (c *remoteConnController) translate() {
	defer close(c.recvCh)
	for ev := range c.conn.Events() {
		env := c.eventToEnvelope(ev)
		if env != nil {
			c.recvCh <- env
		}
	}
}

func (c *remoteConnController) eventToEnvelope(ev control.Event) *proto.Envelope {
	switch ev.Kind {
	case control.KindPeers:
		names := make([]string, 0, len(ev.Peers))
		for _, p := range ev.Peers {
			if p.Name != "" {
				names = append(names, p.Name)
			}
		}
		c.mu.Lock()
		c.peers = names
		c.mu.Unlock()
		return nil
	case control.KindPeerJoin:
		c.mu.Lock()
		c.peers = append(c.peers, ev.Name)
		c.mu.Unlock()
		p, _ := json.Marshal(proto.SystemPayload{Event: proto.EvJoin, Name: ev.Name})
		return &proto.Envelope{Type: proto.TypeSystem, Payload: p}
	case control.KindPeerLeave:
		c.mu.Lock()
		c.peers = removeStr(c.peers, ev.Name)
		c.mu.Unlock()
		p, _ := json.Marshal(proto.SystemPayload{Event: proto.EvLeave, Name: ev.Name})
		return &proto.Envelope{Type: proto.TypeSystem, Payload: p}
	case control.KindKick:
		p, _ := json.Marshal(proto.SystemPayload{Event: proto.EvKick, Name: ev.Name, Message: ev.Reason})
		return &proto.Envelope{Type: proto.TypeSystem, Payload: p}
	case control.KindChat:
		p, _ := json.Marshal(proto.ChatPayload{Text: ev.Text})
		return &proto.Envelope{Type: proto.TypeChat, From: ev.Name, Payload: p}
	case control.KindChatDirect:
		p, _ := json.Marshal(proto.ChatPayload{Text: ev.Text, To: ev.To})
		return &proto.Envelope{Type: proto.TypeChat, From: ev.Name, To: ev.To, Payload: p}
	case control.KindSay:
		p, _ := json.Marshal(proto.ChatPayload{Text: ev.Text})
		return &proto.Envelope{Type: proto.TypeChat, From: "server", Payload: p}
	case control.KindMotd:
		p, _ := json.Marshal(proto.SystemPayload{Event: proto.EvMotd, Message: ev.Text})
		return &proto.Envelope{Type: proto.TypeSystem, Payload: p}
	case control.KindBye:
		p, _ := json.Marshal(proto.KickPayload{Reason: "server disconnected"})
		return &proto.Envelope{Type: proto.TypeKick, Payload: p}
	}
	return nil
}

func removeStr(ss []string, name string) []string {
	out := ss[:0]
	for _, s := range ss {
		if s != name {
			out = append(out, s)
		}
	}
	return out
}

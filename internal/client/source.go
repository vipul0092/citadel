package client

import (
	"encoding/json"
	"fmt"
	"sync"

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

// --- in-process controller ---

type inProcessController struct{ c *Conn }

// NewInProcessController wraps a *Conn as a ConnController.
func NewInProcessController(c *Conn) ConnController { return &inProcessController{c: c} }

func (i *inProcessController) Recv() (*proto.Envelope, error)        { return i.c.Recv() }
func (i *inProcessController) Send(t, to string, p any) error        { return i.c.Send(t, to, p) }
func (i *inProcessController) Name() string                          { return i.c.Name() }
func (i *inProcessController) ServerName() string                    { return i.c.ServerName }
func (i *inProcessController) Motd() string                          { return i.c.Motd }
func (i *inProcessController) Peers() []string                       { return i.c.Peers }
func (i *inProcessController) Close()                                { i.c.Close() }

// --- remote controller ---

type remoteConnController struct {
	conn       *ctrlclient.Conn
	recvCh     chan *proto.Envelope
	myName     string
	serverName string
	mu         sync.Mutex
	peers      []string
}

// NewRemoteController dials the client's control socket and returns a ConnController
// that the dashboard drill-in TUI can use to send/receive as if it were the real Conn.
func NewRemoteController(sockPath, myName, displayServerName string) (ConnController, error) {
	conn, err := ctrlclient.Dial(sockPath)
	if err != nil {
		return nil, err
	}
	_ = conn.Send(map[string]any{"op": "subscribe", "level": "full", "since": 0})
	_ = conn.Send(map[string]any{"op": "list-peers"})

	c := &remoteConnController{
		conn:       conn,
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
	for frame := range c.conn.Events() {
		var m map[string]any
		if err := json.Unmarshal(frame, &m); err != nil {
			continue
		}
		env := c.frameToEnvelope(m)
		if env != nil {
			c.recvCh <- env
		}
	}
}

func (c *remoteConnController) frameToEnvelope(m map[string]any) *proto.Envelope {
	evType, _ := m["ev"].(string)
	name, _ := m["name"].(string)
	text, _ := m["text"].(string)
	to, _ := m["to"].(string)
	reason, _ := m["reason"].(string)

	switch evType {
	case "peers":
		raw, _ := m["peers"].([]any)
		peers := make([]string, 0, len(raw))
		for _, item := range raw {
			pm, _ := item.(map[string]any)
			n, _ := pm["name"].(string)
			if n != "" {
				peers = append(peers, n)
			}
		}
		c.mu.Lock()
		c.peers = peers
		c.mu.Unlock()
		return nil // handled locally

	case "peer-join":
		c.mu.Lock()
		c.peers = append(c.peers, name)
		c.mu.Unlock()
		p, _ := json.Marshal(proto.SystemPayload{Event: proto.EvJoin, Name: name})
		return &proto.Envelope{Type: proto.TypeSystem, Payload: p}

	case "peer-leave":
		c.mu.Lock()
		c.peers = removeStr(c.peers, name)
		c.mu.Unlock()
		p, _ := json.Marshal(proto.SystemPayload{Event: proto.EvLeave, Name: name})
		return &proto.Envelope{Type: proto.TypeSystem, Payload: p}

	case "kick":
		p, _ := json.Marshal(proto.SystemPayload{Event: proto.EvKick, Name: name, Message: reason})
		return &proto.Envelope{Type: proto.TypeSystem, Payload: p}

	case "chat":
		p, _ := json.Marshal(proto.ChatPayload{Text: text})
		return &proto.Envelope{Type: proto.TypeChat, From: name, Payload: p}

	case "chat-direct":
		p, _ := json.Marshal(proto.ChatPayload{Text: text, To: to})
		return &proto.Envelope{Type: proto.TypeChat, From: name, To: to, Payload: p}

	case "say":
		p, _ := json.Marshal(proto.ChatPayload{Text: text})
		return &proto.Envelope{Type: proto.TypeChat, From: "server", Payload: p}

	case "motd-changed":
		p, _ := json.Marshal(proto.SystemPayload{Event: proto.EvMotd, Message: text})
		return &proto.Envelope{Type: proto.TypeSystem, Payload: p}

	case "bye":
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

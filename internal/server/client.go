package server

import (
	"bytes"
	"log/slog"
	"net"
	"time"

	"github.com/vipul0092/citadel/internal/proto"
)

const (
	sendQueueSize = 64
	idleTimeout   = 60 * time.Second
	helloTimeout  = 5 * time.Second
)

// clientConn is the server-side actor for one connected client.
type clientConn struct {
	name        string
	serverName  string // set by the server before calling serve
	conn        net.Conn
	remoteIP    string
	connectedAt time.Time
	hub         *Hub
	send        chan []byte
}

func newClientConn(conn net.Conn, serverName string, hub *Hub) *clientConn {
	host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	return &clientConn{
		serverName:  serverName,
		conn:        conn,
		remoteIP:    host,
		connectedAt: time.Now(),
		hub:         hub,
		send:        make(chan []byte, sendQueueSize),
	}
}

// enqueue puts data on the outbound queue. If the queue is full the connection is closed.
func (c *clientConn) enqueue(data []byte) {
	if data == nil {
		return
	}
	select {
	case c.send <- data:
	default:
		slog.Warn("client send queue full, closing", "name", c.name)
		c.closeConn()
	}
}

func (c *clientConn) closeConn() {
	_ = c.conn.Close()
}

// serve performs the hello/welcome handshake then runs read+write goroutines.
// Blocks until the client disconnects.
func (c *clientConn) serve() {
	defer c.conn.Close()

	// hello handshake with a tight deadline
	if err := c.conn.SetDeadline(time.Now().Add(helloTimeout)); err != nil {
		return
	}
	env, err := proto.Decode(c.conn)
	if err != nil || env.Type != proto.TypeHello {
		return
	}
	var hello proto.HelloPayload
	if err := proto.UnmarshalPayload(env, &hello); err != nil {
		return
	}
	if hello.Version != proto.ProtoVersion {
		_ = proto.Encode(c.conn, proto.TypeReject, "", "", proto.RejectPayload{Reason: "unsupported version"})
		return
	}
	if err := proto.ValidateName(hello.Name); err != nil {
		_ = proto.Encode(c.conn, proto.TypeReject, "", "", proto.RejectPayload{Reason: "invalid name"})
		return
	}
	c.name = hello.Name

	if err := c.conn.SetDeadline(time.Time{}); err != nil {
		return
	}

	// atomic name-uniqueness check + registration
	res := c.hub.Register(c)
	if !res.OK {
		_ = proto.Encode(c.conn, proto.TypeReject, "", "", proto.RejectPayload{Reason: res.Reason})
		return
	}
	if err := proto.Encode(c.conn, proto.TypeWelcome, "", "", proto.WelcomePayload{
		ServerName: c.serverName,
		Motd:       res.Motd,
		Peers:      res.Peers,
	}); err != nil {
		c.hub.Unregister(c)
		return
	}

	done := make(chan struct{})
	go c.writeLoop(done)
	c.readLoop()
	close(done)
	c.hub.Unregister(c)
}

func (c *clientConn) readLoop() {
	for {
		_ = c.conn.SetDeadline(time.Now().Add(idleTimeout))
		env, err := proto.Decode(c.conn)
		if err != nil {
			return
		}
		_ = c.conn.SetDeadline(time.Time{})
		c.handleEnvelope(env)
	}
}

func (c *clientConn) writeLoop(done <-chan struct{}) {
	for {
		select {
		case data := <-c.send:
			if _, err := c.conn.Write(data); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

func (c *clientConn) handleEnvelope(env *proto.Envelope) {
	switch env.Type {
	case proto.TypePing:
		c.enqueue(c.hub.encode(proto.TypePong, "", "", proto.PongPayload{}))

	case proto.TypeChat:
		var p proto.ChatPayload
		if err := proto.UnmarshalPayload(env, &p); err != nil {
			return
		}
		if len(p.Text) > proto.MaxChatMsgLen {
			c.enqueue(c.hub.encode(proto.TypeSystem, "", "", proto.SystemPayload{
				Event: proto.EvTooLong,
				Name:  c.name,
			}))
			return
		}
		var buf bytes.Buffer
		if p.To != "" {
			if err := proto.Encode(&buf, proto.TypeChat, c.name, p.To, p); err == nil {
				c.hub.Direct(p.To, buf.Bytes())
			}
			c.hub.log.Direct(c.name, p.To, p.Text)
			c.hub.emit(HubEvent{Kind: EvDirect, Name: c.name, Target: p.To, Text: p.Text})
		} else {
			if err := proto.Encode(&buf, proto.TypeChat, c.name, "", p); err == nil {
				c.hub.BroadcastRaw(buf.Bytes(), "")
			}
			c.hub.log.Chat(c.name, p.Text)
			c.hub.emit(HubEvent{Kind: EvChat, Name: c.name, Text: p.Text})
		}

	case proto.TypeGame:
		var buf bytes.Buffer
		if err := proto.Encode(&buf, proto.TypeGame, c.name, env.To, env.Payload); err == nil {
			if env.To != "" {
				c.hub.Direct(env.To, buf.Bytes())
			} else {
				c.hub.BroadcastRaw(buf.Bytes(), c.name)
			}
		}

	case proto.TypeLeave:
		c.closeConn()
	}
}

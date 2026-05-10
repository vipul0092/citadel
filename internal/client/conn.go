package client

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"github.com/vipul0092/citadel/internal/proto"
)

const (
	pingInterval     = 15 * time.Second
	dialTimeout      = 10 * time.Second
	probeTimeout     = 300 * time.Millisecond
)

// Conn manages the client's TCP connection to a Citadel server.
type Conn struct {
	addr       string
	myName     string
	ServerName string
	Motd       string
	Peers      []string

	conn    net.Conn
	recvCh  chan *proto.Envelope
	recvErr chan error
	sendCh  chan []byte
	closeCh chan struct{}
}

// Dial opens a TCP connection to addr but does not send hello yet.
func Dial(addr string) (*Conn, error) {
	return dial(addr, dialTimeout)
}

// ProbeLocalhost dials localhost:7777 with a short timeout (used for same-machine fast-path).
func ProbeLocalhost() (*Conn, error) {
	return dial("localhost:7777", probeTimeout)
}

func dial(addr string, timeout time.Duration) (*Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", addr, err)
	}
	return &Conn{
		addr:    addr,
		conn:    conn,
		recvCh:  make(chan *proto.Envelope, 32),
		recvErr: make(chan error, 1),
		sendCh:  make(chan []byte, 64),
		closeCh: make(chan struct{}),
	}, nil
}

// Handshake sends hello{name} and waits for welcome or reject.
// On success it starts background read/write/ping goroutines.
func (c *Conn) Handshake(name string) error {
	if err := proto.Encode(c.conn, proto.TypeHello, "", "", proto.HelloPayload{
		Name:    name,
		Version: proto.ProtoVersion,
	}); err != nil {
		return fmt.Errorf("sending hello: %w", err)
	}

	env, err := proto.Decode(c.conn)
	if err != nil {
		return fmt.Errorf("reading server response: %w", err)
	}

	switch env.Type {
	case proto.TypeWelcome:
		var w proto.WelcomePayload
		if err := proto.UnmarshalPayload(env, &w); err != nil {
			return fmt.Errorf("parsing welcome: %w", err)
		}
		c.myName = name
		c.ServerName = w.ServerName
		c.Motd = w.Motd
		c.Peers = w.Peers
		go c.readLoop()
		go c.writeLoop()
		go c.pingLoop()
		return nil

	case proto.TypeReject:
		var r proto.RejectPayload
		_ = proto.UnmarshalPayload(env, &r)
		return fmt.Errorf("%s", r.Reason)

	default:
		return fmt.Errorf("unexpected message type %q during handshake", env.Type)
	}
}

// Recv blocks until the next inbound envelope or connection error.
func (c *Conn) Recv() (*proto.Envelope, error) {
	select {
	case env := <-c.recvCh:
		return env, nil
	case err := <-c.recvErr:
		return nil, err
	}
}

// Send encodes payload into an Envelope and queues it for delivery.
func (c *Conn) Send(msgType, to string, payload any) error {
	var buf bytes.Buffer
	if err := proto.Encode(&buf, msgType, c.myName, to, payload); err != nil {
		return fmt.Errorf("encoding %s: %w", msgType, err)
	}
	select {
	case c.sendCh <- buf.Bytes():
		return nil
	default:
		return fmt.Errorf("send queue full")
	}
}

// Name returns the client's registered name.
func (c *Conn) Name() string { return c.myName }

// Close sends a leave message and closes the connection.
func (c *Conn) Close() {
	_ = c.Send(proto.TypeLeave, "", proto.LeavePayload{})
	select {
	case <-c.closeCh:
	default:
		close(c.closeCh)
	}
	_ = c.conn.Close()
}

func (c *Conn) readLoop() {
	for {
		env, err := proto.Decode(c.conn)
		if err != nil {
			select {
			case c.recvErr <- err:
			default:
			}
			return
		}
		select {
		case c.recvCh <- env:
		case <-c.closeCh:
			return
		}
	}
}

func (c *Conn) writeLoop() {
	for {
		select {
		case data := <-c.sendCh:
			if _, err := c.conn.Write(data); err != nil {
				return
			}
		case <-c.closeCh:
			return
		}
	}
}

func (c *Conn) pingLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = c.Send(proto.TypePing, "", proto.PingPayload{})
		case <-c.closeCh:
			return
		}
	}
}

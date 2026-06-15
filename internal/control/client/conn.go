// Package client provides a Go client for the citadel control-plane UDS protocol.
// Used by citadel test subcommands and integration tests.
package client

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/vipul0092/citadel/internal/control"
	"github.com/vipul0092/citadel/internal/proto"
)

// HelloEv is the first frame received from the server after dialing.
type HelloEv struct {
	Role    string `json:"role"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Conn is a connected control-plane client. Dial fills Hello; subsequent
// events arrive on Events(). Send delivers ops to the server.
type Conn struct {
	conn   net.Conn
	mu     sync.Mutex // protects writes
	recvCh chan []byte
	done   chan struct{}
	Hello  HelloEv
}

// Dial connects to the UDS socket at sockPath, reads the hello frame, and starts
// a background read goroutine. The caller must call Close() when done.
func Dial(sockPath string) (*Conn, error) {
	nc, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("dial control socket %s: %w", sockPath, err)
	}

	// Read the hello frame that the server sends immediately on connect.
	helloRaw, err := proto.ReadFrame(nc)
	if err != nil {
		_ = nc.Close()
		return nil, fmt.Errorf("read hello: %w", err)
	}
	var hello struct {
		Ev string `json:"ev"`
		HelloEv
	}
	if err := json.Unmarshal(helloRaw, &hello); err != nil || hello.Ev != "hello" {
		_ = nc.Close()
		return nil, fmt.Errorf("unexpected hello frame: %s", helloRaw)
	}

	c := &Conn{
		conn:   nc,
		recvCh: make(chan []byte, 64),
		done:   make(chan struct{}),
		Hello:  hello.HelloEv,
	}
	go c.readLoop()
	return c, nil
}

// Send marshals v to JSON and writes it as a length-prefixed frame.
func (c *Conn) Send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal op: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return proto.WriteFrame(c.conn, data)
}

// Events returns the channel that delivers raw JSON event frames from the server.
// The channel is closed when the connection is closed or the server sends bye.
func (c *Conn) Events() <-chan []byte { return c.recvCh }

// Close stops the read goroutine and closes the underlying connection.
func (c *Conn) Close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	_ = c.conn.Close()
}

func (c *Conn) readLoop() {
	defer close(c.recvCh)
	for {
		frame, err := proto.ReadFrame(c.conn)
		if err != nil {
			return
		}
		select {
		case c.recvCh <- frame:
		case <-c.done:
			return
		}
	}
}

// Subscriber is a subscribed control-plane connection that delivers decoded
// control.Events. Created by DialAndSubscribe; the caller must Close() when done.
type Subscriber struct {
	conn     *Conn
	eventsCh chan control.Event
	Hello    HelloEv
}

// DialAndSubscribe dials sockPath, sends a subscribe op at the given level and
// since-sequence, and returns a Subscriber whose Events channel delivers decoded
// control.Events. The caller must Close() when done.
func DialAndSubscribe(sockPath, level string, since int) (*Subscriber, error) {
	conn, err := Dial(sockPath)
	if err != nil {
		return nil, err
	}
	if err := conn.Send(map[string]any{"op": "subscribe", "level": level, "since": since}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("subscribe: %w", err)
	}
	s := &Subscriber{
		conn:     conn,
		eventsCh: make(chan control.Event, 64),
		Hello:    conn.Hello,
	}
	go s.decode()
	return s, nil
}

// Events returns the channel of decoded control events. Closed when the connection closes.
func (s *Subscriber) Events() <-chan control.Event { return s.eventsCh }

// Send marshals v to JSON and writes it as a length-prefixed frame.
func (s *Subscriber) Send(v any) error { return s.conn.Send(v) }

// Close stops the subscriber and closes the underlying connection.
func (s *Subscriber) Close() { s.conn.Close() }

func (s *Subscriber) decode() {
	defer close(s.eventsCh)
	for frame := range s.conn.Events() {
		ev, err := control.Decode(frame)
		if err != nil {
			continue
		}
		s.eventsCh <- ev
	}
}

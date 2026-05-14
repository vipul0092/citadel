package control

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/vipul0092/citadel/internal/proto"
)

// openListener creates the UDS socket at ~/.citadel/run/<pid>.sock, starts the accept loop,
// and returns the socket path and a stop function.
func openListener(hub *Hub, role, name, version string, actions ActionsProvider) (sockPath string, stop func(), err error) {
	dir, err := RunDir()
	if err != nil {
		return "", nil, err
	}
	sockPath = filepath.Join(dir, fmt.Sprintf("%d.sock", os.Getpid()))

	// Remove stale socket from a previous crash.
	_ = os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return "", nil, fmt.Errorf("listen unix %s: %w", sockPath, err)
	}
	if err := os.Chmod(sockPath, 0o600); err != nil {
		_ = ln.Close()
		return "", nil, fmt.Errorf("chmod socket: %w", err)
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	stop = func() {
		close(stopCh)
		_ = ln.Close()
		_ = os.Remove(sockPath)
		wg.Wait()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-stopCh:
					return
				default:
					slog.Warn("control listener: accept error", "err", err)
					return
				}
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				serveAttacher(conn, hub, role, name, version, actions, stopCh)
			}()
		}
	}()

	return sockPath, stop, nil
}

// serveAttacher handles one control-plane attacher connection.
//
// Protocol sequence:
//  1. Send hello immediately.
//  2. Start a reader goroutine that parses op frames and pushes raw JSON to opsCh.
//  3. Main loop selects over opsCh / active subscription channels / stopCh.
func serveAttacher(conn net.Conn, hub *Hub, role, name, version string, actions ActionsProvider, stopCh <-chan struct{}) {
	defer func() { _ = conn.Close() }()

	// Send hello.
	helloFrame, err := buildSimpleFrame(struct {
		Ev      string `json:"ev"`
		Role    string `json:"role"`
		Name    string `json:"name"`
		Version string `json:"version"`
	}{Ev: "hello", Role: role, Name: name, Version: version})
	if err != nil || proto.WriteFrame(conn, helloFrame) != nil {
		return
	}

	opsCh := make(chan []byte, 16)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			frame, err := proto.ReadFrame(conn)
			if err != nil {
				return
			}
			select {
			case opsCh <- frame:
			case <-stopCh:
				return
			}
		}
	}()

	var s *sub // nil until subscribe op received

	sendError := func(code, msg string) {
		frame, err := buildSimpleFrame(struct {
			Ev   string `json:"ev"`
			Code string `json:"code"`
			Msg  string `json:"msg"`
		}{Ev: "error", Code: code, Msg: msg})
		if err == nil {
			_ = proto.WriteFrame(conn, frame)
		}
	}
	sendEv := func(frame []byte) bool {
		return proto.WriteFrame(conn, frame) == nil
	}

	// replayCh and live channels for the current sub.
	// We use local vars to avoid repeated nil checks in the select.
	var replayCh <-chan replayPkt
	var liveCh <-chan []byte
	var gameCh <-chan []byte

	defer func() {
		// Send bye on all exit paths.
		byeFrame, err := buildSimpleFrame(struct {
			Ev     string `json:"ev"`
			Reason string `json:"reason"`
		}{Ev: "bye", Reason: "shutdown"})
		if err == nil {
			_ = proto.WriteFrame(conn, byeFrame)
		}
		if s != nil {
			hub.Unsubscribe(s)
		}
	}()

	for {
		select {
		case <-stopCh:
			return

		case <-readerDone:
			return

		case raw, ok := <-opsCh:
			if !ok {
				return
			}
			if err := handleOp(raw, hub, conn, actions, &s, &replayCh, &liveCh, &gameCh, sendError); err != nil {
				if err.Error() == "shutdown" {
					return
				}
			}

		case pkt, ok := <-replayCh:
			if !ok {
				replayCh = nil
				continue
			}
			// Deliver gap + replay events + live marker.
			if pkt.gap != nil {
				frame, err := buildSimpleFrame(struct {
					Ev          string `json:"ev"`
					MissingFrom uint64 `json:"missing_from"`
					MissingTo   uint64 `json:"missing_to"`
				}{Ev: "gap", MissingFrom: pkt.gap.MissingFrom, MissingTo: pkt.gap.MissingTo})
				if err != nil || !sendEv(frame) {
					return
				}
			}
			for _, ev := range pkt.events {
				frame, err := buildEventFrame(ev)
				if err != nil || !sendEv(frame) {
					return
				}
			}
			liveFrame, err := buildSimpleFrame(struct {
				Ev string `json:"ev"`
			}{Ev: "live"})
			if err != nil || !sendEv(liveFrame) {
				return
			}
			replayCh = nil // replay delivered; switch to live-only path

		case frame, ok := <-liveCh:
			if !ok {
				return
			}
			if !sendEv(frame) {
				return
			}

		case frame, ok := <-gameCh:
			if !ok {
				gameCh = nil
				continue
			}
			if !sendEv(frame) {
				return
			}
		}
	}
}

// handleOp processes one op frame, updating the subscription state in place.
// Returns an error with message "shutdown" when the attacher requests process shutdown.
func handleOp(
	raw []byte,
	hub *Hub,
	conn net.Conn,
	actions ActionsProvider,
	sp **sub,
	replayCh *<-chan replayPkt,
	liveCh *<-chan []byte,
	gameCh *<-chan []byte,
	sendError func(code, msg string),
) error {
	op, err := peekOp(raw)
	if err != nil {
		sendError("EINVAL", "malformed op: "+err.Error())
		return nil
	}

	setChannels := func(s *sub) {
		*replayCh = s.replayCh
		*liveCh = s.liveCh
		if s.wantGame {
			*gameCh = s.gameCh
		}
	}

	notsup := func(op string) {
		sendError("ENOTSUP", "op not supported for this role: "+op)
	}

	switch op {
	case "subscribe":
		var req subscribeOp
		_ = json.Unmarshal(raw, &req)
		if *sp != nil {
			hub.Unsubscribe(*sp)
		}
		s := hub.Subscribe(parseLevel(req.Level), req.Since, false)
		*sp = s
		setChannels(s)

	case "set-level":
		var req setLevelOp
		_ = json.Unmarshal(raw, &req)
		if *sp == nil {
			sendError("EINVAL", "must subscribe before set-level")
			return nil
		}
		hub.Unsubscribe(*sp)
		s := hub.Subscribe(parseLevel(req.Level), req.Since, false)
		*sp = s
		setChannels(s)

	case "subscribe-game":
		if *sp == nil {
			sendError("EINVAL", "must subscribe before subscribe-game")
			return nil
		}
		hub.SetGameSub(*sp, true)
		*gameCh = (*sp).gameCh

	case "unsubscribe-game":
		if *sp != nil {
			hub.SetGameSub(*sp, false)
		}
		*gameCh = nil

	case "ping":
		frame, err := buildSimpleFrame(struct {
			Ev string `json:"ev"`
		}{Ev: "pong"})
		if err == nil {
			_ = proto.WriteFrame(conn, frame)
		}

	case "shutdown":
		return fmt.Errorf("shutdown")

	case "list-peers":
		if actions == nil {
			notsup(op)
			return nil
		}
		peers := actions.ListPeers()
		frame, err := buildSimpleFrame(struct {
			Ev    string     `json:"ev"`
			Peers []PeerInfo `json:"peers"`
		}{Ev: "peers", Peers: peers})
		if err == nil {
			_ = proto.WriteFrame(conn, frame)
		}

	case "kick":
		if actions == nil {
			notsup(op)
			return nil
		}
		var req struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(raw, &req)
		ok, err := actions.KickPeer(req.Name, req.Reason)
		if err != nil {
			if err == ErrNotSupported {
				notsup(op)
			} else {
				sendError("EINTERNAL", err.Error())
			}
			return nil
		}
		if !ok {
			sendError("ENOENT", "peer not found: "+req.Name)
		}

	case "say":
		if actions == nil {
			notsup(op)
			return nil
		}
		var req struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(raw, &req)
		if err := actions.SayAll(req.Text); err != nil {
			if err == ErrNotSupported {
				notsup(op)
			} else {
				sendError("EINTERNAL", err.Error())
			}
		}

	case "set-motd":
		if actions == nil {
			notsup(op)
			return nil
		}
		var req struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(raw, &req)
		if err := actions.SetMotd(req.Text); err != nil {
			if err == ErrNotSupported {
				notsup(op)
			} else {
				sendError("EINTERNAL", err.Error())
			}
		}

	case "send-chat":
		if actions == nil {
			notsup(op)
			return nil
		}
		var req struct {
			Text string `json:"text"`
			To   string `json:"to"`
		}
		_ = json.Unmarshal(raw, &req)
		if err := actions.SendChat(req.Text, req.To); err != nil {
			if err == ErrNotSupported {
				notsup(op)
			} else {
				sendError("EINTERNAL", err.Error())
			}
		}

	case "send-game":
		if actions == nil {
			notsup(op)
			return nil
		}
		var req struct {
			Kind string          `json:"kind"`
			To   string          `json:"to"`
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(raw, &req)
		if err := actions.SendGame(req.Kind, req.To, req.Data); err != nil {
			if err == ErrNotSupported {
				notsup(op)
			} else {
				sendError("EINTERNAL", err.Error())
			}
		}

	default:
		sendError("EINVAL", "unknown op: "+op)
	}
	return nil
}

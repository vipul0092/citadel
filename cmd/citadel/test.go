package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	ctrlclient "github.com/vipul0092/citadel/internal/control/client"
	"github.com/vipul0092/citadel/internal/control"
)

func runTest(args []string) {
	if len(args) == 0 {
		printTestUsage()
		os.Exit(1)
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "watch":
		runTestWatch(rest)
	case "send-chat":
		runTestSendChat(rest)
	case "send-game":
		runTestSendGame(rest)
	case "kick":
		runTestKick(rest)
	case "drive":
		runTestDrive(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown test subcommand %q\n", sub)
		printTestUsage()
		os.Exit(1)
	}
}

// resolveSock returns a UDS socket path from flag value or session pointer files.
func resolveSock(sockFlag, role string) (string, error) {
	if sockFlag != "" {
		return sockFlag, nil
	}
	if role == "server" {
		if sock, err := control.DefaultServerSock(); err == nil {
			return sock, nil
		}
		// Fall back to scanning sentinels for a running server.
		if sentinels, err := control.ScanSentinels(); err == nil {
			for _, s := range sentinels {
				if s.Role == "server" {
					return s.SockPath, nil
				}
			}
		}
		return "", fmt.Errorf("no active server session; use --sock to specify a socket path")
	}
	// Try client/current.json first, then host/current.json client side.
	if sock, err := control.DefaultClientSock(); err == nil {
		return sock, nil
	}
	if sock, err := control.DefaultHostClientSock(); err == nil {
		return sock, nil
	}
	// Fall back to scanning sentinels for a running client.
	if sentinels, err := control.ScanSentinels(); err == nil {
		for _, s := range sentinels {
			if s.Role == "client" {
				return s.SockPath, nil
			}
		}
	}
	return "", fmt.Errorf("no active session; use --sock to specify a socket path")
}

func dialOrExit(sockFlag, role string) *ctrlclient.Conn {
	sockPath, err := resolveSock(sockFlag, role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	c, err := ctrlclient.Dial(sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return c
}

// runTestWatch subscribes and prints every event to stdout.
func runTestWatch(args []string) {
	fs := flag.NewFlagSet("test watch", flag.ExitOnError)
	sock := fs.String("sock", "", "control socket path")
	role := fs.String("role", "client", "client or server")
	level := fs.String("level", "full", "summary or full")
	_ = fs.Parse(args)

	c := dialOrExit(*sock, *role)
	defer c.Close()

	_ = c.Send(map[string]any{"op": "subscribe", "level": *level, "since": 0})

	for frame := range c.Events() {
		fmt.Println(string(frame))
	}
}

// runTestSendChat sends one chat message and exits.
// --role client (default): sends via the client socket (send-chat op, supports --to).
// --role server: broadcasts via the server socket (say op).
func runTestSendChat(args []string) {
	fs := flag.NewFlagSet("test send-chat", flag.ExitOnError)
	sock := fs.String("sock", "", "control socket path")
	role := fs.String("role", "client", "client or server")
	text := fs.String("text", "", "message text (required)")
	to := fs.String("to", "", "direct recipient (empty = broadcast; client role only)")
	_ = fs.Parse(args)

	if *text == "" {
		fmt.Fprintln(os.Stderr, "error: --text is required")
		os.Exit(1)
	}

	c := dialOrExit(*sock, *role)
	defer c.Close()

	var op map[string]any
	if *role == "server" {
		op = map[string]any{"op": "say", "text": *text}
	} else {
		op = map[string]any{"op": "send-chat", "text": *text, "to": *to}
	}
	if err := c.Send(op); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// runTestSendGame sends one game message and exits.
func runTestSendGame(args []string) {
	fs := flag.NewFlagSet("test send-game", flag.ExitOnError)
	sock := fs.String("sock", "", "control socket path")
	kind := fs.String("kind", "", "game message kind (required)")
	to := fs.String("to", "", "direct recipient (empty = broadcast)")
	data := fs.String("data", "{}", "JSON payload")
	_ = fs.Parse(args)

	if *kind == "" {
		fmt.Fprintln(os.Stderr, "error: --kind is required")
		os.Exit(1)
	}

	var dataRaw json.RawMessage
	if err := json.Unmarshal([]byte(*data), &dataRaw); err != nil {
		fmt.Fprintf(os.Stderr, "error: --data is not valid JSON: %v\n", err)
		os.Exit(1)
	}

	c := dialOrExit(*sock, "client")
	defer c.Close()

	if err := c.Send(map[string]any{"op": "send-game", "kind": *kind, "to": *to, "data": dataRaw}); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// runTestKick kicks a peer via the server socket.
func runTestKick(args []string) {
	fs := flag.NewFlagSet("test kick", flag.ExitOnError)
	sock := fs.String("sock", "", "control socket path (defaults to server socket)")
	name := fs.String("name", "", "peer name to kick (required)")
	reason := fs.String("reason", "kicked by citadel test", "kick reason")
	_ = fs.Parse(args)

	if *name == "" {
		fmt.Fprintln(os.Stderr, "error: --name is required")
		os.Exit(1)
	}

	c := dialOrExit(*sock, "server")
	defer c.Close()

	if err := c.Send(map[string]any{"op": "kick", "name": *name, "reason": *reason}); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// runTestDrive reads newline-delimited JSON ops from stdin and sends them.
// Incoming events are printed to stdout concurrently.
func runTestDrive(args []string) {
	fs := flag.NewFlagSet("test drive", flag.ExitOnError)
	sock := fs.String("sock", "", "control socket path")
	role := fs.String("role", "client", "client or server")
	_ = fs.Parse(args)

	c := dialOrExit(*sock, *role)
	defer c.Close()

	// Print incoming events in the background.
	go func() {
		for frame := range c.Events() {
			fmt.Println(string(frame))
		}
	}()

	// Read ops from stdin and send each line.
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Validate JSON before sending.
		if !json.Valid([]byte(line)) {
			fmt.Fprintf(os.Stderr, "skip (invalid JSON): %s\n", line)
			continue
		}
		if err := c.Send(json.RawMessage(line)); err != nil {
			fmt.Fprintf(os.Stderr, "send error: %v\n", err)
			return
		}
	}
}

func printTestUsage() {
	fmt.Fprintf(os.Stderr, `Usage: citadel test <subcommand> [flags]

Subcommands:
  watch      Subscribe and print events (Ctrl-C to stop)
  send-chat  Send a chat message
  send-game  Send a game payload
  kick       Kick a peer (uses server socket)
  drive      Read newline-JSON ops from stdin and send them

Common flags:
  --sock  Control socket path (default: from active session pointer file)
  --role  client or server (affects default socket; default: client)

watch flags:
  --level  summary or full (default: full)

send-chat flags:
  --role  client (default) or server
  --text  message text (required)
  --to    direct recipient (empty = broadcast; client role only)

send-game flags:
  --kind  game message kind (required)
  --to    direct recipient (empty = broadcast)
  --data  JSON payload (default: {})

kick flags:
  --name    peer name to kick (required)
  --reason  kick reason
`)
}

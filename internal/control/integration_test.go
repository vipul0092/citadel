//go:build integration

package control

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	ctrlclient "github.com/vipul0092/citadel/internal/control/client"
)

// testBin is the path to the compiled citadel binary used by integration tests.
var testBin string

func TestMain(m *testing.M) {
	// Prefer the pre-built binary from `mise run build` (at module root / two dirs up).
	abs, err := filepath.Abs("../../citadel")
	if err == nil {
		if _, err := os.Stat(abs); err == nil {
			testBin = abs
		}
	}

	if testBin == "" {
		// Fall back to building it now.
		tmp, err := os.CreateTemp("", "citadel-integration-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "integration: cannot create temp file: %v\n", err)
			os.Exit(1)
		}
		_ = tmp.Close()
		testBin = tmp.Name()
		defer os.Remove(testBin)

		build := exec.Command("go", "build", "-o", testBin, "../../cmd/citadel")
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "integration: cannot build citadel: %v\n", err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

// spawnProc starts a citadel subprocess, registers a SIGTERM+Wait cleanup, and returns the Cmd.
func spawnProc(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(testBin, args...)
	cmd.Stderr = os.Stderr // surface stderr from subprocess in test output
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn %v: %v", args, err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			_ = cmd.Wait()
		}
	})
	return cmd
}

// waitSentinel calls WaitForSentinel with a 5 s timeout, failing the test on timeout.
func waitSentinel(t *testing.T, role, name string) *SentinelInfo {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := WaitForSentinel(ctx, role, name)
	if err != nil {
		t.Fatalf("wait sentinel (role=%s name=%s): %v", role, name, err)
	}
	return info
}

// drainUntil reads from ch until predicate returns true or timeout expires.
func drainUntil(t *testing.T, ch <-chan []byte, timeout time.Duration, predicate func(map[string]any) bool) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case frame, ok := <-ch:
			if !ok {
				t.Fatal("events channel closed before predicate matched")
			}
			var m map[string]any
			if err := json.Unmarshal(frame, &m); err != nil {
				continue
			}
			if predicate(m) {
				return
			}
		case <-deadline:
			t.Fatalf("timeout waiting for event matching predicate")
		}
	}
}

const intTestPort = "19877"

// TestServerControlPlane verifies:
//   - server sentinel appears after start
//   - control socket accepts attacher (hello with correct role/name)
//   - subscribe delivers live marker
//   - client join triggers peer-join event on server subscriber
//   - list-peers returns the connected client
func TestServerControlPlane(t *testing.T) {
	// Use /tmp as base: macOS UNIX_PATH_MAX=104; t.TempDir() produces paths >104 chars.
	tmpHome, err := os.MkdirTemp("/tmp", "cit-itest-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpHome) })
	t.Setenv("HOME", tmpHome)

	// Spawn server.
	spawnProc(t, "server", "--headless", "--name", "inttest-srv", "--port", intTestPort)
	srvInfo := waitSentinel(t, "server", "inttest-srv")

	// Dial server control socket.
	sc, err := ctrlclient.Dial(srvInfo.SockPath)
	if err != nil {
		t.Fatalf("dial server control: %v", err)
	}
	defer sc.Close()

	if sc.Hello.Role != "server" {
		t.Errorf("hello.role: want server, got %s", sc.Hello.Role)
	}
	if sc.Hello.Name != "inttest-srv" {
		t.Errorf("hello.name: want inttest-srv, got %s", sc.Hello.Name)
	}
	if sc.Hello.Version == "" {
		t.Error("hello.version: empty")
	}

	// Subscribe at full level.
	if err := sc.Send(map[string]any{"op": "subscribe", "level": "full", "since": 0}); err != nil {
		t.Fatalf("send subscribe: %v", err)
	}
	// Drain until live marker.
	drainUntil(t, sc.Events(), 5*time.Second, func(m map[string]any) bool {
		return m["ev"] == "live"
	})

	// Spawn a client that connects to the server.
	spawnProc(t, "connect", "--headless",
		"--server", "localhost:"+intTestPort, "--name", "alice")
	waitSentinel(t, "client", "alice")

	// Expect peer-join for alice.
	drainUntil(t, sc.Events(), 5*time.Second, func(m map[string]any) bool {
		return m["ev"] == "peer-join" && m["name"] == "alice"
	})

	// list-peers should include alice.
	if err := sc.Send(map[string]any{"op": "list-peers"}); err != nil {
		t.Fatalf("send list-peers: %v", err)
	}
	drainUntil(t, sc.Events(), 5*time.Second, func(m map[string]any) bool {
		if m["ev"] != "peers" {
			return false
		}
		peers, _ := m["peers"].([]any)
		for _, p := range peers {
			pm, _ := p.(map[string]any)
			if pm["name"] == "alice" {
				return true
			}
		}
		return false
	})
}

// TestClientSendChat verifies that a send-chat op on the client control socket
// results in a chat event on the server subscriber.
//
// Subscribe to the server BEFORE spawning the client so that peer-join arrives
// as a live event (not in the replay). This avoids drainUntil(live) consuming
// the peer-join frame before we check for it.
func TestClientSendChat(t *testing.T) {
	// Use /tmp as base: macOS UNIX_PATH_MAX=104; t.TempDir() produces paths >104 chars.
	tmpHome, err := os.MkdirTemp("/tmp", "cit-itest-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpHome) })
	t.Setenv("HOME", tmpHome)

	// Spawn server and wait for its sentinel.
	spawnProc(t, "server", "--headless", "--name", "inttest-srv2", "--port", intTestPort)
	srvInfo := waitSentinel(t, "server", "inttest-srv2")

	// Subscribe to the server control plane BEFORE spawning the client.
	// This ensures peer-join arrives as a live event (not in the replay batch).
	sc, err := ctrlclient.Dial(srvInfo.SockPath)
	if err != nil {
		t.Fatalf("dial server control: %v", err)
	}
	defer sc.Close()
	if err := sc.Send(map[string]any{"op": "subscribe", "level": "full", "since": 0}); err != nil {
		t.Fatalf("subscribe server: %v", err)
	}
	drainUntil(t, sc.Events(), 5*time.Second, func(m map[string]any) bool {
		return m["ev"] == "live"
	})

	// Now spawn the client — peer-join will be a live event on sc.
	spawnProc(t, "connect", "--headless",
		"--server", "localhost:"+intTestPort, "--name", "bob")
	cliInfo := waitSentinel(t, "client", "bob")

	// Wait for peer-join (live event).
	drainUntil(t, sc.Events(), 5*time.Second, func(m map[string]any) bool {
		return m["ev"] == "peer-join" && m["name"] == "bob"
	})

	// Dial CLIENT control and send a chat message.
	cc, err := ctrlclient.Dial(cliInfo.SockPath)
	if err != nil {
		t.Fatalf("dial client control: %v", err)
	}
	defer cc.Close()
	if err := cc.Send(map[string]any{"op": "send-chat", "text": "hello from test"}); err != nil {
		t.Fatalf("send-chat: %v", err)
	}

	// Expect chat event on the server subscriber.
	drainUntil(t, sc.Events(), 5*time.Second, func(m map[string]any) bool {
		return m["ev"] == "chat" && m["text"] == "hello from test"
	})
}

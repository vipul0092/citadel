package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSentinelLifecycle(t *testing.T) {
	// Use a temp dir instead of ~/.citadel/run to avoid polluting the real state.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sent, err := WriteSentinel("server", "test-srv", "127.0.0.1:7777", "/tmp/test.sock", "test-srv", []string{"server", "--name", "test-srv"})
	if err != nil {
		t.Fatalf("WriteSentinel: %v", err)
	}

	// File must exist.
	data, err := os.ReadFile(sent.Path)
	if err != nil {
		t.Fatalf("sentinel file not found: %v", err)
	}

	// Must be valid JSON with required fields.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("sentinel is not valid JSON: %v", err)
	}
	for _, field := range []string{"pid", "role", "name", "addr", "control", "args", "started"} {
		if _, ok := m[field]; !ok {
			t.Errorf("sentinel missing field %q", field)
		}
	}
	if m["role"] != "server" {
		t.Errorf("role: want server, got %v", m["role"])
	}
	if m["name"] != "test-srv" {
		t.Errorf("name: want test-srv, got %v", m["name"])
	}

	// Unlink must remove the file.
	sent.Unlink()
	if _, err := os.Stat(sent.Path); !os.IsNotExist(err) {
		t.Error("sentinel file should be removed after Unlink")
	}
}

func TestSentinelRunDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir, err := RunDir()
	if err != nil {
		t.Fatalf("RunDir: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
	// Mode should be 0700 (user only).
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("run dir perm: want 0700, got %04o", perm)
	}

	expected := filepath.Join(tmp, ".citadel", "run")
	if dir != expected {
		t.Errorf("run dir: want %s, got %s", expected, dir)
	}
}

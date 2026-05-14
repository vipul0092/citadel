package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClientPointerLifecycle(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	ptr, err := WriteClientPointer(
		"/tmp/12345.sock",
		"192.168.1.5:7777",
		"Throne",
		"Aarav",
	)
	if err != nil {
		t.Fatalf("WriteClientPointer: %v", err)
	}

	// File must exist at expected path.
	expected := filepath.Join(tmp, ".citadel", "client", "current.json")
	if ptr.Path != expected {
		t.Errorf("path: want %s, got %s", expected, ptr.Path)
	}

	data, err := os.ReadFile(ptr.Path)
	if err != nil {
		t.Fatalf("read pointer file: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	for _, field := range []string{"client_sock", "client_pid", "server_addr", "server_name", "my_name", "started_at"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing field %q", field)
		}
	}
	if m["server_name"] != "Throne" {
		t.Errorf("server_name: want Throne, got %v", m["server_name"])
	}
	if m["my_name"] != "Aarav" {
		t.Errorf("my_name: want Aarav, got %v", m["my_name"])
	}

	// Unlink removes the file.
	ptr.Unlink()
	if _, err := os.Stat(ptr.Path); !os.IsNotExist(err) {
		t.Error("pointer file should be removed after Unlink")
	}

	// Second Unlink is a no-op.
	ptr.Unlink()
}

func TestClientPointerNilUnlink(t *testing.T) {
	var p *ClientPointer
	p.Unlink() // must not panic
}

func TestHostPointerLifecycle(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	server := &SentinelInfo{Pid: 100, Name: "Throne", SockPath: "/tmp/100.sock"}
	client := &SentinelInfo{Pid: 101, SockPath: "/tmp/101.sock"}

	ptr, err := WriteHostPointer(server, client, "Vipul")
	if err != nil {
		t.Fatalf("WriteHostPointer: %v", err)
	}

	expected := filepath.Join(tmp, ".citadel", "host", "current.json")
	if ptr.Path != expected {
		t.Errorf("path: want %s, got %s", expected, ptr.Path)
	}

	data, err := os.ReadFile(ptr.Path)
	if err != nil {
		t.Fatalf("read host pointer: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, field := range []string{"server_sock", "server_pid", "server_name", "client_sock", "client_pid", "my_name", "started_at"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing field %q", field)
		}
	}
	if m["my_name"] != "Vipul" {
		t.Errorf("my_name: want Vipul, got %v", m["my_name"])
	}
	if m["server_name"] != "Throne" {
		t.Errorf("server_name: want Throne, got %v", m["server_name"])
	}

	ptr.Unlink()
	if _, err := os.Stat(ptr.Path); !os.IsNotExist(err) {
		t.Error("host pointer should be removed after Unlink")
	}
}

func TestHostPointerNilUnlink(t *testing.T) {
	var p *HostPointer
	p.Unlink() // must not panic
}

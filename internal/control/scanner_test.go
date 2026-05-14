package control

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestReadSentinel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := filepath.Join(tmp, ".citadel", "run")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(sentinelJSON{
		Pid:     42,
		Role:    "server",
		Name:    "Throne",
		Addr:    "192.168.1.5:7777",
		Control: "/tmp/42.sock",
		Args:    []string{"server", "--name", "Throne"},
		Started: "2026-05-14T10:00:00Z",
	})
	path := filepath.Join(dir, "42.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := ReadSentinel(path)
	if err != nil {
		t.Fatalf("ReadSentinel: %v", err)
	}
	if info.Pid != 42 {
		t.Errorf("Pid: want 42, got %d", info.Pid)
	}
	if info.Role != "server" {
		t.Errorf("Role: want server, got %s", info.Role)
	}
	if info.Name != "Throne" {
		t.Errorf("Name: want Throne, got %s", info.Name)
	}
	if info.SockPath != "/tmp/42.sock" {
		t.Errorf("SockPath: want /tmp/42.sock, got %s", info.SockPath)
	}
}

func TestScanSentinels(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := filepath.Join(tmp, ".citadel", "run")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Write a sentinel with the current (alive) pid.
	alivePid := os.Getpid()
	aliveRaw, _ := json.Marshal(sentinelJSON{
		Pid: alivePid, Role: "server", Name: "alive",
		Control: "/tmp/alive.sock",
	})
	if err := os.WriteFile(
		filepath.Join(dir, fmt.Sprintf("%d.json", alivePid)),
		aliveRaw, 0o600,
	); err != nil {
		t.Fatal(err)
	}

	// Write a sentinel with an obviously stale pid.
	stalePid := math.MaxInt32
	staleRaw, _ := json.Marshal(sentinelJSON{
		Pid: stalePid, Role: "server", Name: "stale",
		Control: "/tmp/stale.sock",
	})
	if err := os.WriteFile(
		filepath.Join(dir, fmt.Sprintf("%d.json", stalePid)),
		staleRaw, 0o600,
	); err != nil {
		t.Fatal(err)
	}

	results, err := ScanSentinels()
	if err != nil {
		t.Fatalf("ScanSentinels: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 live sentinel, got %d", len(results))
	}
	if results[0].Name != "alive" {
		t.Errorf("expected alive sentinel, got %s", results[0].Name)
	}
}

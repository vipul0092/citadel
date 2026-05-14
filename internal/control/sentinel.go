package control

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sentinel represents the per-process discovery file written to ~/.citadel/run/<pid>.json.
type Sentinel struct {
	Path string
}

type sentinelJSON struct {
	Pid        int      `json:"pid"`
	Role       string   `json:"role"`
	Name       string   `json:"name"`
	Addr       string   `json:"addr"`
	Control    string   `json:"control"`
	Args       []string `json:"args"`
	Started    string   `json:"started"`
	ServerName string   `json:"server_name,omitempty"`
}

// WriteSentinel creates ~/.citadel/run/<pid>.json and returns the Sentinel handle.
// sockPath is the UDS socket path that was already opened by openListener.
// args should be os.Args[1:].
func WriteSentinel(role, name, addr, sockPath, serverName string, args []string) (*Sentinel, error) {
	dir, err := RunDir()
	if err != nil {
		return nil, err
	}
	pid := os.Getpid()
	path := filepath.Join(dir, fmt.Sprintf("%d.json", pid))

	s := sentinelJSON{
		Pid:        pid,
		Role:       role,
		Name:       name,
		Addr:       addr,
		Control:    sockPath,
		Args:       args,
		Started:    time.Now().UTC().Format(time.RFC3339),
		ServerName: serverName,
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal sentinel: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write sentinel %s: %w", path, err)
	}
	return &Sentinel{Path: path}, nil
}

// Unlink removes the sentinel file. Safe to call multiple times.
func (s *Sentinel) Unlink() {
	_ = os.Remove(s.Path)
}

// RunDir returns the path to ~/.citadel/run/, creating it if necessary.
func RunDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".citadel", "run")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return dir, nil
}

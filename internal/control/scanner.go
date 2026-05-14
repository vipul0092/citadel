package control

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// SentinelInfo is the publicly readable representation of one process's sentinel file.
type SentinelInfo struct {
	Pid        int
	Role       string
	Name       string
	Addr       string
	SockPath   string   // the "control" field — path to the UDS socket
	Started    string
	Args       []string // original argv[1:]; used by dashboard [r] Restart
	ServerName string   // server display name; set for client role
}

// ReadSentinel parses a single sentinel JSON file and returns a SentinelInfo.
func ReadSentinel(path string) (*SentinelInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sentinel %s: %w", path, err)
	}
	var s sentinelJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse sentinel %s: %w", path, err)
	}
	return &SentinelInfo{
		Pid:        s.Pid,
		Role:       s.Role,
		Name:       s.Name,
		Addr:       s.Addr,
		SockPath:   s.Control,
		Started:    s.Started,
		Args:       s.Args,
		ServerName: s.ServerName,
	}, nil
}

// ScanSentinels reads all sentinel files in ~/.citadel/run/ and returns live entries.
// Entries whose pid is no longer alive are silently skipped.
func ScanSentinels() ([]SentinelInfo, error) {
	dir, err := RunDir()
	if err != nil {
		return nil, err
	}
	entries, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob sentinels: %w", err)
	}

	var result []SentinelInfo
	for _, path := range entries {
		info, err := ReadSentinel(path)
		if err != nil {
			continue // skip unreadable / malformed files
		}
		if !pidAlive(info.Pid) {
			continue // stale sentinel — skip (dashboard will clean it up in P6)
		}
		result = append(result, *info)
	}
	return result, nil
}

// WaitForSentinel polls ScanSentinels every 200 ms until a live sentinel matching
// role and name appears or ctx expires.
func WaitForSentinel(ctx context.Context, role, name string) (*SentinelInfo, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for sentinel (role=%s name=%s): %w", role, name, ctx.Err())
		case <-ticker.C:
			sentinels, err := ScanSentinels()
			if err != nil {
				continue // transient error; keep polling
			}
			for i := range sentinels {
				if sentinels[i].Role == role && sentinels[i].Name == name {
					return &sentinels[i], nil
				}
			}
		}
	}
}

// pidAlive reports whether the process with the given pid is alive.
// It uses kill(pid, 0), which succeeds if the process exists and we have permission to signal it.
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

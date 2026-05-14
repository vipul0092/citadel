package control

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ClientPointer represents the session pointer file written by `citadel connect --headless`.
// Path is ~/.citadel/client/current.json.
type ClientPointer struct {
	Path string
}

type clientPointerJSON struct {
	ClientSock string `json:"client_sock"`
	ClientPid  int    `json:"client_pid"`
	ServerAddr string `json:"server_addr"`
	ServerName string `json:"server_name"`
	MyName     string `json:"my_name"`
	StartedAt  string `json:"started_at"`
}

// WriteClientPointer creates ~/.citadel/client/current.json with the session's socket paths
// and connection metadata. Returns nil pointer (not an error) if sockPath is empty.
func WriteClientPointer(sockPath, serverAddr, serverName, myName string) (*ClientPointer, error) {
	dir, err := clientDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "current.json")

	p := clientPointerJSON{
		ClientSock: sockPath,
		ClientPid:  os.Getpid(),
		ServerAddr: serverAddr,
		ServerName: serverName,
		MyName:     myName,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal client pointer: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write client pointer %s: %w", path, err)
	}
	return &ClientPointer{Path: path}, nil
}

// Unlink removes the pointer file. Safe to call multiple times or on a nil pointer.
func (p *ClientPointer) Unlink() {
	if p == nil {
		return
	}
	_ = os.Remove(p.Path)
}

// clientDir returns the path to ~/.citadel/client/, creating it if necessary.
func clientDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".citadel", "client")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return dir, nil
}

// HostPointer represents the session pointer file written by `citadel host`.
// Path is ~/.citadel/host/current.json.
type HostPointer struct {
	Path string
}

type hostPointerJSON struct {
	ServerSock string `json:"server_sock"`
	ServerPid  int    `json:"server_pid"`
	ServerName string `json:"server_name"`
	ClientSock string `json:"client_sock"`
	ClientPid  int    `json:"client_pid"`
	MyName     string `json:"my_name"`
	StartedAt  string `json:"started_at"`
}

// WriteHostPointer creates ~/.citadel/host/current.json referencing both the server and
// client processes that make up the host session.
func WriteHostPointer(server, client *SentinelInfo, myName string) (*HostPointer, error) {
	dir, err := hostDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "current.json")

	p := hostPointerJSON{
		ServerSock: server.SockPath,
		ServerPid:  server.Pid,
		ServerName: server.Name,
		ClientSock: client.SockPath,
		ClientPid:  client.Pid,
		MyName:     myName,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal host pointer: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write host pointer %s: %w", path, err)
	}
	return &HostPointer{Path: path}, nil
}

// Unlink removes the host pointer file. Safe to call multiple times or on a nil pointer.
func (p *HostPointer) Unlink() {
	if p == nil {
		return
	}
	_ = os.Remove(p.Path)
}

// DefaultClientSock returns the client UDS socket path from ~/.citadel/client/current.json.
func DefaultClientSock() (string, error) {
	dir, err := clientDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		return "", fmt.Errorf("no active client session: %w", err)
	}
	var p clientPointerJSON
	if err := json.Unmarshal(data, &p); err != nil {
		return "", fmt.Errorf("parse client pointer: %w", err)
	}
	return p.ClientSock, nil
}

// DefaultServerSock returns the server UDS socket path from ~/.citadel/host/current.json.
func DefaultServerSock() (string, error) {
	dir, err := hostDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		return "", fmt.Errorf("no active host session: %w", err)
	}
	var p hostPointerJSON
	if err := json.Unmarshal(data, &p); err != nil {
		return "", fmt.Errorf("parse host pointer: %w", err)
	}
	return p.ServerSock, nil
}

// DefaultHostClientSock returns the client UDS socket path from ~/.citadel/host/current.json.
func DefaultHostClientSock() (string, error) {
	dir, err := hostDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		return "", fmt.Errorf("no active host session: %w", err)
	}
	var p hostPointerJSON
	if err := json.Unmarshal(data, &p); err != nil {
		return "", fmt.Errorf("parse host pointer: %w", err)
	}
	return p.ClientSock, nil
}

// hostDir returns the path to ~/.citadel/host/, creating it if necessary.
func hostDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".citadel", "host")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return dir, nil
}

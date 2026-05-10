package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ActivityLog writes JSON-lines event records to disk.
// All methods are safe for concurrent use.
type ActivityLog struct {
	mu sync.Mutex
	f  *os.File
}

type logRecord struct {
	Time  string `json:"time"`
	Event string `json:"event"`
	Name  string `json:"name,omitempty"`
	Text  string `json:"text,omitempty"`
}

// OpenActivityLog opens (or creates) the log at path.
// Passing an empty path disables disk logging.
func OpenActivityLog(path string) (*ActivityLog, error) {
	if path == "" {
		return &ActivityLog{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening activity log: %w", err)
	}
	return &ActivityLog{f: f}, nil
}

func (l *ActivityLog) write(event, name, text string) {
	if l.f == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	rec := logRecord{
		Time:  time.Now().Format(time.RFC3339),
		Event: event,
		Name:  name,
		Text:  text,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		slog.Error("marshaling log record", "err", err)
		return
	}
	if _, err := fmt.Fprintf(l.f, "%s\n", data); err != nil {
		slog.Error("writing activity log", "err", err)
	}
}

func (l *ActivityLog) Join(name string)                { l.write("join", name, "") }
func (l *ActivityLog) Leave(name string)               { l.write("leave", name, "") }
func (l *ActivityLog) Kick(name, reason string)        { l.write("kick", name, reason) }
func (l *ActivityLog) Chat(name, text string)          { l.write("chat", name, text) }
func (l *ActivityLog) Direct(from, to, text string)    { l.write("direct", from, to+": "+text) }
func (l *ActivityLog) Say(text string)                 { l.write("say", "server", text) }
func (l *ActivityLog) Motd(text string)                { l.write("motd", "server", text) }

func (l *ActivityLog) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		_ = l.f.Close()
	}
}

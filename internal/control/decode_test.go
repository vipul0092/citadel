package control

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDecode_PeerJoin(t *testing.T) {
	frame, _ := buildEventFrame(ringEntry{Seq: 1, At: time.Now(), Kind: "peer-join", Data: rawJSON(`{"name":"Vipul"}`)})
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindPeerJoin {
		t.Errorf("kind: want %q, got %q", KindPeerJoin, ev.Kind)
	}
	if ev.Name != "Vipul" {
		t.Errorf("name: want Vipul, got %q", ev.Name)
	}
}

func TestDecode_PeerLeave(t *testing.T) {
	frame, _ := buildEventFrame(ringEntry{Seq: 1, At: time.Now(), Kind: "peer-leave", Data: rawJSON(`{"name":"Aarav"}`)})
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindPeerLeave {
		t.Errorf("kind: want %q, got %q", KindPeerLeave, ev.Kind)
	}
	if ev.Name != "Aarav" {
		t.Errorf("name: want Aarav, got %q", ev.Name)
	}
}

func TestDecode_Kick(t *testing.T) {
	data, _ := json.Marshal(struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}{"Maya", "spamming"})
	frame, _ := buildEventFrame(ringEntry{Seq: 1, At: time.Now(), Kind: "kick", Data: data})
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindKick {
		t.Errorf("kind: want %q, got %q", KindKick, ev.Kind)
	}
	if ev.Name != "Maya" {
		t.Errorf("name: want Maya, got %q", ev.Name)
	}
	if ev.Reason != "spamming" {
		t.Errorf("reason: want spamming, got %q", ev.Reason)
	}
}

func TestDecode_Chat(t *testing.T) {
	data, _ := json.Marshal(struct {
		Name string `json:"name"`
		Text string `json:"text"`
	}{"Vipul", "hello"})
	frame, _ := buildEventFrame(ringEntry{Seq: 1, At: time.Now(), Kind: "chat", Data: data})
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindChat {
		t.Errorf("kind: want %q, got %q", KindChat, ev.Kind)
	}
	if ev.Name != "Vipul" || ev.Text != "hello" {
		t.Errorf("got name=%q text=%q", ev.Name, ev.Text)
	}
}

func TestDecode_ChatDirect(t *testing.T) {
	data, _ := json.Marshal(struct {
		Name string `json:"name"`
		To   string `json:"to"`
		Text string `json:"text"`
	}{"Vipul", "Aarav", "psst"})
	frame, _ := buildEventFrame(ringEntry{Seq: 1, At: time.Now(), Kind: "chat-direct", Data: data})
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindChatDirect {
		t.Errorf("kind: want %q, got %q", KindChatDirect, ev.Kind)
	}
	if ev.Name != "Vipul" || ev.To != "Aarav" || ev.Text != "psst" {
		t.Errorf("got name=%q to=%q text=%q", ev.Name, ev.To, ev.Text)
	}
}

func TestDecode_Peers(t *testing.T) {
	connectedStr := "2024-01-02T03:04:05Z"
	frame, _ := buildSimpleFrame(struct {
		Ev    string     `json:"ev"`
		Peers []PeerInfo `json:"peers"`
	}{
		Ev:    "peers",
		Peers: []PeerInfo{{Name: "Vipul", IP: "10.0.0.1", Connected: connectedStr}},
	})
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindPeers {
		t.Errorf("kind: want %q, got %q", KindPeers, ev.Kind)
	}
	if len(ev.Peers) != 1 {
		t.Fatalf("peers: want 1, got %d", len(ev.Peers))
	}
	p := ev.Peers[0]
	if p.Name != "Vipul" {
		t.Errorf("peer name: want Vipul, got %q", p.Name)
	}
	if p.IP != "10.0.0.1" {
		t.Errorf("peer ip: want 10.0.0.1, got %q", p.IP)
	}
	if p.Connected != connectedStr {
		t.Errorf("peer connected: want %q, got %q", connectedStr, p.Connected)
	}
}

func TestDecode_Live(t *testing.T) {
	frame, _ := buildSimpleFrame(struct {
		Ev string `json:"ev"`
	}{"live"})
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindLive {
		t.Errorf("kind: want %q, got %q", KindLive, ev.Kind)
	}
}

func TestDecode_Bye(t *testing.T) {
	frame, _ := buildSimpleFrame(struct {
		Ev     string `json:"ev"`
		Reason string `json:"reason"`
	}{"bye", "shutdown"})
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindBye {
		t.Errorf("kind: want %q, got %q", KindBye, ev.Kind)
	}
	if ev.Reason != "shutdown" {
		t.Errorf("reason: want shutdown, got %q", ev.Reason)
	}
}

func TestDecode_Game(t *testing.T) {
	frame, _ := buildGameFrame("Vipul", "move", "Aarav", rawJSON(`{"x":3,"y":4}`))
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindGame {
		t.Errorf("kind: want %q, got %q", KindGame, ev.Kind)
	}
	if ev.Name != "Vipul" {
		t.Errorf("name (sender): want Vipul, got %q", ev.Name)
	}
	if ev.GameKind != "move" {
		t.Errorf("game kind: want move, got %q", ev.GameKind)
	}
	if ev.To != "Aarav" {
		t.Errorf("to: want Aarav, got %q", ev.To)
	}
	if string(ev.Data) != `{"x":3,"y":4}` {
		t.Errorf("data: want {\"x\":3,\"y\":4}, got %s", ev.Data)
	}
}

func TestDecode_UnknownKind(t *testing.T) {
	frame, _ := buildSimpleFrame(struct {
		Ev string `json:"ev"`
	}{"future-event-type"})
	ev, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != EventKind("future-event-type") {
		t.Errorf("kind: want future-event-type, got %q", ev.Kind)
	}
}

func TestDecode_MalformedJSON(t *testing.T) {
	_, err := Decode([]byte(`not json`))
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

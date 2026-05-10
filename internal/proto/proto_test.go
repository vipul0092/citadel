package proto_test

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/vipul0092/citadel/internal/proto"
)

func TestFrameRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })

	want := []byte("hello world")
	errc := make(chan error, 1)
	go func() { errc <- proto.WriteFrame(client, want) }()

	got, err := proto.ReadFrame(server)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFrameTooBig(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })

	big := make([]byte, proto.MaxFrameSize+1)
	if err := proto.WriteFrame(client, big); err == nil {
		t.Error("expected error for oversized frame")
	}
	_ = server
}

func TestEnvelopeRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })

	payload := proto.ChatPayload{Text: "test message"}
	errc := make(chan error, 1)
	go func() {
		errc <- proto.Encode(client, proto.TypeChat, "Vipul", "", payload)
	}()

	env, err := proto.Decode(server)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if env.Type != proto.TypeChat {
		t.Errorf("type = %q, want %q", env.Type, proto.TypeChat)
	}
	if env.From != "Vipul" {
		t.Errorf("from = %q, want Vipul", env.From)
	}

	var got proto.ChatPayload
	if err := proto.UnmarshalPayload(env, &got); err != nil {
		t.Fatalf("UnmarshalPayload: %v", err)
	}
	if got.Text != payload.Text {
		t.Errorf("text = %q, want %q", got.Text, payload.Text)
	}
}

func TestFrameZeroLength(t *testing.T) {
	var buf bytes.Buffer
	if err := proto.WriteFrame(&buf, []byte{}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got, err := proto.ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d bytes, want 0", len(got))
	}
}

func TestReadFrameCorruptHeader(t *testing.T) {
	// Advertise a huge length that exceeds MaxFrameSize.
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], proto.MaxFrameSize+1)
	buf.Write(hdr[:])

	_, err := proto.ReadFrame(&buf)
	if err == nil {
		t.Fatal("expected error for oversized frame header")
	}
}

func TestDecodeInvalidJSON(t *testing.T) {
	// Write a valid frame containing invalid JSON.
	var buf bytes.Buffer
	if err := proto.WriteFrame(&buf, []byte("{not json")); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	_, err := proto.Decode(&buf)
	if err == nil {
		t.Fatal("expected error decoding invalid JSON")
	}
}

func TestEnvelopeDirectMessage(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })

	payload := proto.ChatPayload{Text: "secret", To: "Bob"}
	errc := make(chan error, 1)
	go func() {
		errc <- proto.Encode(client, proto.TypeChat, "Alice", "Bob", payload)
	}()

	env, err := proto.Decode(server)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if env.To != "Bob" {
		t.Errorf("to = %q, want Bob", env.To)
	}
	if env.From != "Alice" {
		t.Errorf("from = %q, want Alice", env.From)
	}

	var got proto.ChatPayload
	if err := proto.UnmarshalPayload(env, &got); err != nil {
		t.Fatalf("UnmarshalPayload: %v", err)
	}
	if got.To != "Bob" {
		t.Errorf("payload.To = %q, want Bob", got.To)
	}
}

func TestMultipleFramesSequence(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })

	msgs := []string{"hello", "world", "test"}
	errc := make(chan error, 1)
	go func() {
		for _, m := range msgs {
			if err := proto.Encode(client, proto.TypeChat, "user", "", proto.ChatPayload{Text: m}); err != nil {
				errc <- err
				return
			}
		}
		errc <- nil
	}()

	var seqs []uint64
	for range msgs {
		env, err := proto.Decode(server)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		seqs = append(seqs, env.Seq)
	}
	if err := <-errc; err != nil {
		t.Fatalf("Encode: %v", err)
	}

	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Errorf("seq not monotonically increasing: %d <= %d", seqs[i], seqs[i-1])
		}
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"Vipul", false},
		{"user_123", false},
		{"a-b", false},
		{"A", false},
		{"", true},
		{"this-name-is-way-too-long-to-be-accepted-xyz", true},
		{"has space", true},
		{"has!bang", true},
		{"emoji🎮", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := proto.ValidateName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) err=%v, wantErr=%v", tt.name, err, tt.wantErr)
			}
		})
	}
}

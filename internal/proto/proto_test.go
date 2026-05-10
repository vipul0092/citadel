package proto_test

import (
	"net"
	"testing"

	"github.com/vipul0092/citadel/internal/proto"
)

func TestFrameRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })

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
	t.Cleanup(func() { client.Close(); server.Close() })

	big := make([]byte, proto.MaxFrameSize+1)
	if err := proto.WriteFrame(client, big); err == nil {
		t.Error("expected error for oversized frame")
	}
	_ = server
}

func TestEnvelopeRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })

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

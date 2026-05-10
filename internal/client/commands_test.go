package client

import (
	"testing"
)

func TestParsePlainChat(t *testing.T) {
	cmd := Parse("hello world")
	if cmd.Kind != CmdNone {
		t.Errorf("kind = %d, want CmdNone", cmd.Kind)
	}
	if cmd.Text != "hello world" {
		t.Errorf("text = %q, want %q", cmd.Text, "hello world")
	}
}

func TestParseEmpty(t *testing.T) {
	cmd := Parse("")
	if cmd.Kind != CmdNone {
		t.Errorf("kind = %d, want CmdNone", cmd.Kind)
	}
	if cmd.Text != "" {
		t.Errorf("text = %q, want empty", cmd.Text)
	}
}

func TestParseWho(t *testing.T) {
	cmd := Parse("/who")
	if cmd.Kind != CmdWho {
		t.Errorf("kind = %d, want CmdWho", cmd.Kind)
	}
}

func TestParseQuit(t *testing.T) {
	cmd := Parse("/quit")
	if cmd.Kind != CmdQuit {
		t.Errorf("kind = %d, want CmdQuit", cmd.Kind)
	}
}

func TestParseHelp(t *testing.T) {
	cmd := Parse("/help")
	if cmd.Kind != CmdHelp {
		t.Errorf("kind = %d, want CmdHelp", cmd.Kind)
	}
}

func TestParseMsgValid(t *testing.T) {
	cmd := Parse("/msg Alice hey there")
	if cmd.Kind != CmdMsg {
		t.Fatalf("kind = %d, want CmdMsg", cmd.Kind)
	}
	if cmd.Target != "Alice" {
		t.Errorf("target = %q, want Alice", cmd.Target)
	}
	if cmd.Text != "hey there" {
		t.Errorf("text = %q, want %q", cmd.Text, "hey there")
	}
}

func TestParseMsgMissingText(t *testing.T) {
	cmd := Parse("/msg Alice")
	if cmd.Kind != CmdNone {
		t.Errorf("kind = %d, want CmdNone (usage error)", cmd.Kind)
	}
	if cmd.Text == "" {
		t.Error("expected usage error text")
	}
}

func TestParseMsgMissingArgs(t *testing.T) {
	cmd := Parse("/msg")
	if cmd.Kind != CmdNone {
		t.Errorf("kind = %d, want CmdNone (usage error)", cmd.Kind)
	}
}

func TestParseUnknownCommand(t *testing.T) {
	cmd := Parse("/dance")
	if cmd.Kind != CmdNone {
		t.Errorf("kind = %d, want CmdNone", cmd.Kind)
	}
	if cmd.Text == "" {
		t.Error("expected unknown command error text")
	}
}

func TestParseCaseInsensitive(t *testing.T) {
	cmd := Parse("/WHO")
	if cmd.Kind != CmdWho {
		t.Errorf("kind = %d, want CmdWho (case-insensitive)", cmd.Kind)
	}
}

func TestParseWhitespace(t *testing.T) {
	cmd := Parse("   /quit   ")
	if cmd.Kind != CmdQuit {
		t.Errorf("kind = %d, want CmdQuit after trimming", cmd.Kind)
	}
}

package completer

import (
	"testing"
)

var (
	commands = []string{"/help", "/kick", "/msg", "/quit"}
	nameCmds = []string{"/kick", "/msg"}
	names    = []string{"Alice", "Bob", "Alicia"}
)

func TestComputeNoSlash(t *testing.T) {
	got := Compute("hello", commands, nameCmds, names)
	if got != nil {
		t.Errorf("non-slash input should return nil, got %v", got)
	}
}

func TestComputePartialCommand(t *testing.T) {
	got := Compute("/h", commands, nameCmds, names)
	if len(got) != 1 || got[0] != "/help" {
		t.Errorf("got %v, want [/help]", got)
	}
}

func TestComputePartialCommandMultiple(t *testing.T) {
	// /k matches /kick only, but / matches everything except exact
	got := Compute("/", commands, nameCmds, names)
	if len(got) != 4 {
		t.Errorf("got %d matches, want 4", len(got))
	}
}

func TestComputeExactCommandNoMatch(t *testing.T) {
	// Exact match should not suggest itself
	got := Compute("/quit", commands, nameCmds, names)
	if len(got) != 0 {
		t.Errorf("exact match should return empty, got %v", got)
	}
}

func TestComputeNameCompletion(t *testing.T) {
	got := Compute("/kick al", commands, nameCmds, names)
	if len(got) != 2 {
		t.Fatalf("got %v, want [Alice Alicia]", got)
	}
}

func TestComputeNameCompletionNoMatch(t *testing.T) {
	got := Compute("/kick Zed", commands, nameCmds, names)
	if len(got) != 0 {
		t.Errorf("got %v, want []", got)
	}
}

func TestComputeNonNameCommand(t *testing.T) {
	// /help is not a name command — second arg should not trigger name completion
	got := Compute("/help al", commands, nameCmds, names)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestComputeThreePartsReturnsNil(t *testing.T) {
	got := Compute("/msg Alice hello there", commands, nameCmds, names)
	if got != nil {
		t.Errorf("three-part input should return nil, got %v", got)
	}
}

func TestCompleteCommand(t *testing.T) {
	got := Complete("/h", []string{"/help"}, 0)
	if got != "/help " {
		t.Errorf("got %q, want %q", got, "/help ")
	}
}

func TestCompleteName(t *testing.T) {
	got := Complete("/kick al", []string{"Alice", "Alicia"}, 1)
	if got != "/kick Alicia " {
		t.Errorf("got %q, want %q", got, "/kick Alicia ")
	}
}

func TestCompleteEmptySuggestions(t *testing.T) {
	got := Complete("/foo", nil, 0)
	if got != "/foo" {
		t.Errorf("got %q, want %q", got, "/foo")
	}
}

func TestExtraHeightEmpty(t *testing.T) {
	if h := ExtraHeight(nil); h != 0 {
		t.Errorf("got %d, want 0", h)
	}
}

func TestExtraHeightWithSuggestions(t *testing.T) {
	// 3 suggestions + 1 hint line = 4
	if h := ExtraHeight([]string{"a", "b", "c"}); h != 4 {
		t.Errorf("got %d, want 4", h)
	}
}

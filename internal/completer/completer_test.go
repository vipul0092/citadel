package completer

import (
	"strings"
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

func TestBarEmpty(t *testing.T) {
	got := Bar(nil, 0, 80)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestBarRendersAll(t *testing.T) {
	got := Bar([]string{"/help", "/quit"}, 0, 80)
	if !strings.Contains(got, "/help") || !strings.Contains(got, "/quit") {
		t.Errorf("bar should contain both suggestions, got %q", got)
	}
}

func TestBarSelectedMarker(t *testing.T) {
	got := Bar([]string{"/help", "/quit"}, 0, 80)
	if !strings.Contains(got, "▶") {
		t.Error("bar should contain ▶ marker for selected item")
	}
}

func TestBarOverflow(t *testing.T) {
	suggs := []string{"/help", "/msg", "/quit", "/who"}
	// Use a very narrow width to force truncation
	got := Bar(suggs, 0, 20)
	if !strings.Contains(got, "…") {
		t.Errorf("narrow bar should show overflow indicator, got %q", got)
	}
	if !strings.Contains(got, "(+") {
		t.Errorf("narrow bar should show remaining count, got %q", got)
	}
}

func TestBarIdxWraps(t *testing.T) {
	// idx beyond range should wrap to 0
	got := Bar([]string{"/help", "/quit"}, 5, 80)
	if !strings.Contains(got, "▶") {
		t.Error("bar should still render with wrapped index")
	}
}

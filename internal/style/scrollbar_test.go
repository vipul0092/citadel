package style

import (
	"strings"
	"testing"
)

func TestScrollbarContentFits(t *testing.T) {
	// When content fits, should return blank column
	sb := Scrollbar(10, 5, 0.0)
	lines := strings.Split(sb, "\n")
	if len(lines) != 10 {
		t.Fatalf("got %d lines, want 10", len(lines))
	}
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			t.Errorf("line %d = %q, want blank", i, l)
		}
	}
}

func TestScrollbarAtTop(t *testing.T) {
	sb := Scrollbar(10, 100, 0.0)
	lines := strings.Split(sb, "\n")
	if len(lines) != 10 {
		t.Fatalf("got %d lines, want 10", len(lines))
	}
	// Thumb should start at position 0
	if !strings.Contains(lines[0], "█") {
		t.Error("thumb should be at the top when scrollPercent=0")
	}
}

func TestScrollbarAtBottom(t *testing.T) {
	sb := Scrollbar(10, 100, 1.0)
	lines := strings.Split(sb, "\n")
	if len(lines) != 10 {
		t.Fatalf("got %d lines, want 10", len(lines))
	}
	// Thumb should end at the last line
	if !strings.Contains(lines[len(lines)-1], "█") {
		t.Error("thumb should be at the bottom when scrollPercent=1.0")
	}
}

func TestScrollbarMiddle(t *testing.T) {
	sb := Scrollbar(10, 100, 0.5)
	lines := strings.Split(sb, "\n")
	thumbLines := 0
	for _, l := range lines {
		if strings.Contains(l, "█") {
			thumbLines++
		}
	}
	if thumbLines == 0 {
		t.Error("expected at least one thumb line")
	}
	if thumbLines >= 10 {
		t.Error("thumb should not fill entire track")
	}
}

func TestScrollbarZeroHeight(t *testing.T) {
	sb := Scrollbar(0, 100, 0.5)
	if sb != "" {
		t.Errorf("zero height should return empty string, got %q", sb)
	}
}

func TestScrollbarThumbMinSize(t *testing.T) {
	// With a huge total, thumb should be at least 1 line
	sb := Scrollbar(5, 10000, 0.5)
	lines := strings.Split(sb, "\n")
	thumbLines := 0
	for _, l := range lines {
		if strings.Contains(l, "█") {
			thumbLines++
		}
	}
	if thumbLines < 1 {
		t.Error("thumb should be at least 1 line")
	}
}

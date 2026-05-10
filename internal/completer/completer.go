package completer

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// Compute returns matching completions for input.
//
// commands: all slash commands for this context (e.g. ["/kick", "/quit"])
// nameCmds: subset whose first argument is a peer/client name (e.g. ["/kick"])
// names:    available names to complete against
func Compute(input string, commands, nameCmds, names []string) []string {
	if !strings.HasPrefix(input, "/") {
		return nil
	}
	parts := strings.SplitN(input, " ", 3)
	if len(parts) >= 3 {
		return nil
	}
	if len(parts) == 2 {
		base := strings.ToLower(parts[0])
		for _, nc := range nameCmds {
			if nc == base {
				partial := strings.ToLower(parts[1])
				var out []string
				for _, n := range names {
					if strings.HasPrefix(strings.ToLower(n), partial) {
						out = append(out, n)
					}
				}
				return out
			}
		}
		return nil
	}
	low := strings.ToLower(input)
	var out []string
	for _, c := range commands {
		if strings.HasPrefix(c, low) && c != low {
			out = append(out, c)
		}
	}
	return out
}

// Complete returns the new input value after applying the selected suggestion.
func Complete(input string, suggestions []string, idx int) string {
	if len(suggestions) == 0 {
		return input
	}
	if idx >= len(suggestions) {
		idx = 0
	}
	sugg := suggestions[idx]
	parts := strings.SplitN(input, " ", 2)
	if len(parts) == 2 {
		return parts[0] + " " + sugg + " "
	}
	return sugg + " "
}

// Lines returns styled rows to embed above the input line inside the input box.
// Returns nil when suggestions is empty.
func Lines(suggestions []string, idx int) []string {
	if len(suggestions) == 0 {
		return nil
	}
	if idx >= len(suggestions) {
		idx = 0
	}
	rows := make([]string, len(suggestions)+1)
	for i, s := range suggestions {
		if i == idx {
			rows[i] = selectedStyle.Render("▶ " + s)
		} else {
			rows[i] = normalStyle.Render("  " + s)
		}
	}
	rows[len(suggestions)] = hintStyle.Render("  Tab complete · ↑↓ navigate")
	return rows
}

// ExtraHeight returns the extra content lines added to the input box when
// suggestions are visible (suggestion rows + hint line).
func ExtraHeight(suggestions []string) int {
	if len(suggestions) == 0 {
		return 0
	}
	return len(suggestions) + 1
}

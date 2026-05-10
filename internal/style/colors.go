package style

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
)

// palette is a set of perceptually distinct ANSI-256 colors.
// Deliberately avoids the cyan/teal/blue range (86, 87, 79, 75, 62…) used by
// the UI chrome, so no peer name can accidentally look like server text.
// Hues span: coral → orange → gold → yellow → lime → green → lavender → pink → rose.
var palette = []lipgloss.Color{
	"203", // coral
	"209", // salmon
	"214", // orange
	"220", // gold
	"226", // yellow
	"154", // lime green
	"46",  // bright green
	"41",  // emerald
	"141", // lavender
	"165", // purple
	"213", // hot pink
	"204", // rose
}

// ColorForName returns a deterministic, distinct color for a given name.
// The same name always maps to the same color.
func ColorForName(name string) lipgloss.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return palette[int(h.Sum32())%len(palette)]
}

// NameStyle returns a bold lipgloss style in the name's assigned color.
func NameStyle(name string) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(ColorForName(name))
}

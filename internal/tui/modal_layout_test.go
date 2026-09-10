package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestJoinModalColumnsFillsUnevenRowsWithModalBackground(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	got := joinModalColumns("aa\nbb", "ccc")
	lines := strings.Split(stripANSI(got), "\n")
	if len(lines) != 2 {
		t.Fatalf("joined modal has %d rows, want 2: %q", len(lines), got)
	}
	if lines[0] != "aaccc" || lines[1] != "bb   " {
		t.Fatalf("joined modal text = %q, want %q", lines, []string{"aaccc", "bb   "})
	}

	fill := modalFillStyle.Render(" ")
	fillPrefixEnd := strings.IndexByte(fill, ' ')
	if fillPrefixEnd < 0 || !strings.Contains(got, fill[:fillPrefixEnd]) {
		t.Fatalf("joined modal did not style padding with modal background %q: %q", fill, got)
	}
}

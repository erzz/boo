package picker

import (
	"strings"
	"testing"
)

// truncateToWidth must never produce a string longer than `width`
// runes — wrapping the status bar onto a second line breaks the
// statusBarHeight=1 layout math.
func TestTruncateToWidth_ShortInputUnchanged(t *testing.T) {
	if got := truncateToWidth("hi", 10); got != "hi" {
		t.Errorf("truncateToWidth(\"hi\", 10) = %q, want \"hi\"", got)
	}
}

func TestTruncateToWidth_LongInputClipped(t *testing.T) {
	got := truncateToWidth("abcdefghij", 5)
	if r := []rune(got); len(r) != 5 {
		t.Errorf("truncateToWidth output rune count = %d, want 5 (got %q)", len(r), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix on truncated output, got %q", got)
	}
}

func TestTruncateToWidth_ZeroWidthReturnsInput(t *testing.T) {
	// Pre-WindowSizeMsg state: m.width is 0; we should not panic
	// or produce empty output that hides the message entirely.
	if got := truncateToWidth("anything", 0); got != "anything" {
		t.Errorf("truncateToWidth with width=0 should return input unchanged, got %q", got)
	}
}

func TestTruncateToWidth_UnicodePreservesRunes(t *testing.T) {
	// Multibyte runes (✓, ✖) must not be split mid-byte.
	got := truncateToWidth("✓ deleted alpha", 5)
	if r := []rune(got); len(r) != 5 {
		t.Errorf("rune count = %d, want 5 (got %q)", len(r), got)
	}
}

// clipToHeight must guarantee the right pane never exceeds its inner
// height — splitMinHeight=24 is only a heuristic, layouts with deep
// column splits could still produce content taller than the available
// rows.
func TestClipToHeight_ShortInputUnchanged(t *testing.T) {
	in := "a\nb\nc"
	if got := clipToHeight(in, 5); got != in {
		t.Errorf("clipToHeight unchanged for short input: got %q want %q", got, in)
	}
}

func TestClipToHeight_LongInputTrimmedWithEllipsis(t *testing.T) {
	in := "line1\nline2\nline3\nline4\nline5"
	got := clipToHeight(in, 3)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Errorf("clipToHeight produced %d lines, want 3 (got %q)", len(lines), got)
	}
	if lines[len(lines)-1] != "…" {
		t.Errorf("last line should be ellipsis to signal truncation, got %q", lines[len(lines)-1])
	}
}

func TestClipToHeight_ZeroHeightReturnsInput(t *testing.T) {
	// Pre-WindowSizeMsg state. Don't blank out content.
	if got := clipToHeight("a\nb", 0); got != "a\nb" {
		t.Errorf("clipToHeight with height=0 should return input unchanged, got %q", got)
	}
}

func TestClipToHeight_HeightOneCollapsesToEllipsis(t *testing.T) {
	// height=1 with multi-line input: a single ellipsis is the only
	// signal we have room for.
	if got := clipToHeight("a\nb\nc", 1); got != "…" {
		t.Errorf("clipToHeight(_, 1) = %q, want \"…\"", got)
	}
}

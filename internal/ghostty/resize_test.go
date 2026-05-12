package ghostty

import "testing"

// Pinning ResizeDeltaPixels behaviour because the JXA walker, the editor
// preview, and any future bounds-aware caller all read from this helper.
// Drift here would silently misrender every sized layout.
func TestResizeDeltaPixels(t *testing.T) {
	cases := []struct {
		name           string
		parentExtentPx int
		targetFrac     float64
		want           int
	}{
		// Even (no-op) — saves an action.
		{"target=0.5 returns zero", 1200, 0.5, 0},
		// Common ratios at a typical window width.
		{"0.7 of 1200 → +240", 1200, 0.7, 240},
		{"0.3 of 1200 → -240", 1200, 0.3, -240},
		// Extremes that ResizeDeltaPixels still emits — the JXA walker
		// is responsible for clamping at the divider's minimum, not us.
		{"0.95 of 1200 → +540", 1200, 0.95, 540},
		{"0.05 of 1200 → -540", 1200, 0.05, -540},
		// Small windows still produce non-zero deltas.
		{"0.6 of 100 → +10", 100, 0.6, 10},
		// Out-of-range or unusable → 0 (caller skips).
		{"target=0 returns zero", 1200, 0.0, 0},
		{"target=1 returns zero", 1200, 1.0, 0},
		{"target=-0.2 returns zero", 1200, -0.2, 0},
		{"target=1.5 returns zero", 1200, 1.5, 0},
		{"zero extent returns zero", 0, 0.7, 0},
		{"negative extent returns zero", -100, 0.7, 0},
		// Sub-pixel deltas round to 0 → no-op.
		{"sub-pixel positive rounds away", 1, 0.51, 0},
		{"sub-pixel negative rounds away", 1, 0.49, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResizeDeltaPixels(c.parentExtentPx, c.targetFrac)
			if got != c.want {
				t.Errorf("ResizeDeltaPixels(%d, %v) = %d, want %d", c.parentExtentPx, c.targetFrac, got, c.want)
			}
		})
	}
}

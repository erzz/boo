package doctor

import (
	"strings"
	"testing"
)

// TestCompareVersions exercises the dotted-decimal comparator used by the
// Ghostty version check.
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.3.0", "1.3.0", 0},
		{"1.2.9", "1.3.0", -1},
		{"1.3.0", "1.2.9", 1},
		{"1.3.1", "1.3.0", 1},
		{"1.3.0", "1.3.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.9.9", "2.0.0", -1},
		{"1.3", "1.3.0", 0}, // shorter segment treated as 0
		{"1.3.0", "1.3", 0},
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		if got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestCheckVersionRange covers the four relevant cases: older-than-min,
// in-range, newer-than-tested, malformed (empty), and a boundary case.
func TestCheckVersionRange(t *testing.T) {
	cases := []struct {
		name       string
		ver        string
		wantStatus Status
		wantSubstr string // substring expected in Detail
	}{
		{
			name:       "empty version is FAIL",
			ver:        "",
			wantStatus: Fail,
			wantSubstr: "empty",
		},
		{
			name:       "too old is FAIL",
			ver:        "1.2.9",
			wantStatus: Fail,
			wantSubstr: "older than minimum",
		},
		{
			name:       "below new minimum is FAIL",
			ver:        "1.3.0",
			wantStatus: Fail,
			wantSubstr: "older than minimum",
		},
		{
			name:       "exactly at min is OK",
			ver:        supportedGhosttyMin,
			wantStatus: OK,
			wantSubstr: supportedGhosttyMin,
		},
		{
			name:       "inside range is OK",
			ver:        "1.3.5",
			wantStatus: OK,
			wantSubstr: "1.3.5",
		},
		{
			name:       "just below upper bound is OK",
			ver:        "1.99.99",
			wantStatus: OK,
			wantSubstr: "1.99.99",
		},
		{
			name:       "exactly at unsupported floor is WARN",
			ver:        unsupportedGhosttyFrom,
			wantStatus: Warn,
			wantSubstr: unsupportedGhosttyFrom,
		},
		{
			name:       "above unsupported floor is WARN",
			ver:        "2.1.0",
			wantStatus: Warn,
			wantSubstr: "2.0.0",
		},
		{
			name:       "malformed non-numeric segment parses as 0",
			ver:        "1.4.0-rc1",
			wantStatus: OK, // "1.4.0" part is in range; "-rc1" parses as 0
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := checkVersionRange(c.ver)
			if r.Status != c.wantStatus {
				t.Errorf("Status = %v, want %v (Detail: %q)", r.Status, c.wantStatus, r.Detail)
			}
			if c.wantSubstr != "" && !strings.Contains(r.Detail, c.wantSubstr) {
				t.Errorf("Detail %q does not contain %q", r.Detail, c.wantSubstr)
			}
		})
	}
}

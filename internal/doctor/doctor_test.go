package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"

	booexec "github.com/erzz/boo/internal/exec"
	"github.com/erzz/boo/internal/ghostty"
)

// ─── compareVersions ─────────────────────────────────────────────────────────

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

// ─── checkVersionRange ───────────────────────────────────────────────────────

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

// ─── ghosttyVersionRangeCheck wiring ─────────────────────────────────────────

// newVersionClient builds a *ghostty.Client whose runner returns versionJSON for every osascript call.
// Pass json="" + non-nil runErr to simulate broken/unavailable Ghostty.
func newVersionClient(versionJSON string, runErr error) *ghostty.Client {
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		if runErr != nil {
			return nil, nil, runErr
		}
		return []byte(versionJSON), nil, nil
	})
	return ghostty.New(fake)
}

// priorWithRunning returns a prior []Result with a "ghostty running" entry at the given status.
func priorWithRunning(s Status) []Result {
	return []Result{{Name: "ghostty running", Status: s, Detail: "test stub"}}
}

// TestGhosttyVersionRangeCheck_SkipPaths: every skip branch must return Skip without calling client.Version().
func TestGhosttyVersionRangeCheck_SkipPaths(t *testing.T) {
	// A client that panics if its runner is called, so we know the skip
	// branches truly never reach the network/osascript.
	neverCall := booexec.NewFake(func(name string, _ []string, _ []byte) ([]byte, []byte, error) {
		panic("runner must not be called on skip paths, but got: " + name)
	})
	client := ghostty.New(neverCall)

	cases := []struct {
		name  string
		prior []Result
	}{
		{
			name:  "no prior result → Skip",
			prior: nil,
		},
		{
			name:  "ghostty running = Fail → Skip",
			prior: priorWithRunning(Fail),
		},
		{
			name:  "ghostty running = Skip → Skip",
			prior: priorWithRunning(Skip),
		},
		{
			name:  "ghostty running = Warn (not running yet) → Skip",
			prior: priorWithRunning(Warn),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check := ghosttyVersionRangeCheck(client)
			r := check(context.Background(), c.prior)
			if r.Status != Skip {
				t.Errorf("Status = %v, want Skip (Detail: %q)", r.Status, r.Detail)
			}
		})
	}
}

// TestGhosttyVersionRangeCheck_ClientError: Ghostty client error → Fail result mentioning the error.
func TestGhosttyVersionRangeCheck_ClientError(t *testing.T) {
	client := newVersionClient("", errors.New("osascript: Application can't be found"))
	check := ghosttyVersionRangeCheck(client)
	r := check(context.Background(), priorWithRunning(OK))

	if r.Status != Fail {
		t.Errorf("Status = %v, want Fail", r.Status)
	}
	if !strings.Contains(r.Detail, "could not read version") {
		t.Errorf("Detail %q should mention 'could not read version'", r.Detail)
	}
}

// TestGhosttyVersionRangeCheck_VersionRouting: in-range → OK; below min → Fail; ≥2.0 → Warn.
func TestGhosttyVersionRangeCheck_VersionRouting(t *testing.T) {
	cases := []struct {
		name       string
		ver        string
		wantStatus Status
		wantSubstr string
	}{
		{
			name:       "in-range version → OK",
			ver:        "1.3.5",
			wantStatus: OK,
			wantSubstr: "1.3.5",
		},
		{
			name:       "exactly at minimum → OK",
			ver:        supportedGhosttyMin,
			wantStatus: OK,
			wantSubstr: supportedGhosttyMin,
		},
		{
			name:       "below minimum → Fail",
			ver:        "1.2.0",
			wantStatus: Fail,
			wantSubstr: "older than minimum",
		},
		{
			name:       "at or above 2.0 → Warn",
			ver:        "2.0.0",
			wantStatus: Warn,
			wantSubstr: "2.0.0",
		},
		{
			name:       "well above 2.0 → Warn",
			ver:        "3.1.4",
			wantStatus: Warn,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Build a client that returns the test version.
			versionJSON := `{"version":"` + c.ver + `"}`
			client := newVersionClient(versionJSON, nil)

			check := ghosttyVersionRangeCheck(client)
			r := check(context.Background(), priorWithRunning(OK))

			if r.Status != c.wantStatus {
				t.Errorf("Status = %v, want %v (Detail: %q)", r.Status, c.wantStatus, r.Detail)
			}
			if c.wantSubstr != "" && !strings.Contains(r.Detail, c.wantSubstr) {
				t.Errorf("Detail %q does not contain %q", r.Detail, c.wantSubstr)
			}
		})
	}
}

// TestGhosttyVersionRangeCheck_CheckNameIsStable: result Name is always "ghostty version" on every path.
// previousResult / AllChecks rely on this name to locate the result.
func TestGhosttyVersionRangeCheck_CheckNameIsStable(t *testing.T) {
	// Test through every code path.
	paths := []struct {
		desc   string
		client *ghostty.Client
		prior  []Result
	}{
		{
			desc:   "skip path",
			client: ghostty.New(booexec.NewFake(nil)),
			prior:  nil,
		},
		{
			desc:   "ok path",
			client: newVersionClient(`{"version":"1.3.5"}`, nil),
			prior:  priorWithRunning(OK),
		},
		{
			desc:   "error path",
			client: newVersionClient("", errors.New("boom")),
			prior:  priorWithRunning(OK),
		},
	}
	for _, p := range paths {
		t.Run(p.desc, func(t *testing.T) {
			check := ghosttyVersionRangeCheck(p.client)
			r := check(context.Background(), p.prior)
			if r.Name != "ghostty version" {
				t.Errorf("Name = %q, want 'ghostty version'", r.Name)
			}
		})
	}
}

// ─── AllChecks composition / wiring ──────────────────────────────────────────

// TestAllChecks_IncludesGhosttyVersionCheck: AllChecks() must include ghosttyVersionRangeCheck
// (catches refactors that accidentally remove it from the chain).
func TestAllChecks_IncludesGhosttyVersionCheck(t *testing.T) {
	// Build a client with a valid in-range version. Even if earlier checks skip the version check,
	// the Result still carries Name="ghostty version".
	client := newVersionClient(`{"version":"1.3.5"}`, nil)

	checks := AllChecks(client)
	results, _ := Run(context.Background(), checks)

	for _, r := range results {
		if r.Name == "ghostty version" {
			return // found — wiring is intact
		}
	}
	t.Errorf("AllChecks() result set does not contain a result with Name='ghostty version'; ghosttyVersionRangeCheck may have been removed from the chain")
}


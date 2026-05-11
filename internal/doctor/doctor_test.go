package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
// Pass versionJSON="" + non-nil runErr to simulate broken/unavailable Ghostty.
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
	client := newVersionClient(`{"version":"1.3.5"}`, nil)

	checks := AllChecks(client)
	results, _ := Run(context.Background(), checks)

	for _, r := range results {
		if r.Name == "ghostty version" {
			return // wiring intact
		}
	}
	t.Errorf("AllChecks() result set does not contain a result with Name='ghostty version'; ghosttyVersionRangeCheck may have been removed from the chain")
}

// ─── Run dispatcher ───────────────────────────────────────────────────────────

// TestRun_AggregatesResults: all checks run in order; worst non-Skip status returned.
func TestRun_AggregatesResults(t *testing.T) {
	checks := []CheckFunc{
		func(_ context.Context, _ []Result) Result { return Result{Name: "a", Status: OK} },
		func(_ context.Context, _ []Result) Result { return Result{Name: "b", Status: Warn} },
		func(_ context.Context, _ []Result) Result { return Result{Name: "c", Status: Fail} },
	}
	results, worst := Run(context.Background(), checks)
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[0].Name != "a" || results[1].Name != "b" || results[2].Name != "c" {
		t.Errorf("unexpected order: %v", results)
	}
	if worst != Fail {
		t.Errorf("worst = %v, want Fail", worst)
	}
}

// TestRun_SkipNotWorst: Skip results do not change the worst-status aggregate.
func TestRun_SkipNotWorst(t *testing.T) {
	checks := []CheckFunc{
		func(_ context.Context, _ []Result) Result { return Result{Name: "a", Status: OK} },
		func(_ context.Context, _ []Result) Result { return Result{Name: "b", Status: Skip} },
	}
	_, worst := Run(context.Background(), checks)
	if worst != OK {
		t.Errorf("worst = %v, want OK (Skip must not count as worst)", worst)
	}
}

// TestRun_LaterCheckSeesEarlierResults: each check receives the accumulated results so far.
func TestRun_LaterCheckSeesEarlierResults(t *testing.T) {
	var priorAtSecond []Result
	checks := []CheckFunc{
		func(_ context.Context, _ []Result) Result { return Result{Name: "first", Status: Warn} },
		func(_ context.Context, prior []Result) Result {
			priorAtSecond = prior
			return Result{Name: "second", Status: OK}
		},
	}
	Run(context.Background(), checks)
	if len(priorAtSecond) != 1 || priorAtSecond[0].Name != "first" {
		t.Errorf("second check received %v, want [{first Warn ...}]", priorAtSecond)
	}
}

// ─── legacyDataDirCheck ───────────────────────────────────────────────────────

// TestLegacyDataDirCheck: warn when old boo state exists; silent OK when HOME is clean.
func TestLegacyDataDirCheck(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(dir string) // creates files under dir (used as HOME)
		wantStatus Status
	}{
		{
			name:       "no legacy state → OK",
			setup:      func(string) {},
			wantStatus: OK,
		},
		{
			name: "projects.toml present → Warn",
			setup: func(dir string) {
				legacyDir := filepath.Join(dir, ".local", "share", "boo")
				if err := os.MkdirAll(legacyDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(legacyDir, "projects.toml"), []byte(""), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantStatus: Warn,
		},
		{
			name: "projects/ subdir present → Warn",
			setup: func(dir string) {
				if err := os.MkdirAll(filepath.Join(dir, ".local", "share", "boo", "projects"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantStatus: Warn,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("XDG_DATA_HOME", "") // ensure HOME-derived path is used
			c.setup(dir)
			check := legacyDataDirCheck()
			r := check(context.Background(), nil)
			if r.Status != c.wantStatus {
				t.Errorf("Status = %v, want %v (Detail: %q)", r.Status, c.wantStatus, r.Detail)
			}
		})
	}
}

// TestLegacyDataDirCheck_XDGOverride: XDG_DATA_HOME is respected when set.
func TestLegacyDataDirCheck_XDGOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	legacyDir := filepath.Join(dir, "boo")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "projects.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	check := legacyDataDirCheck()
	r := check(context.Background(), nil)
	if r.Status != Warn {
		t.Errorf("Status = %v, want Warn (XDG_DATA_HOME not respected)", r.Status)
	}
}

// ─── ConfigCheck ─────────────────────────────────────────────────────────────

// TestConfigCheck: missing → Skip, bad content → Fail, good content → OK.
func TestConfigCheck(t *testing.T) {
	cases := []struct {
		name       string
		write      string // non-empty means create the file with this content
		loadErr    error
		wantStatus Status
	}{
		{
			name:       "missing file → Skip",
			wantStatus: Skip,
		},
		{
			name:       "present and valid → OK",
			write:      "key: val",
			wantStatus: OK,
		},
		{
			name:       "present but load fails → Fail",
			write:      "key: val",
			loadErr:    errors.New("parse error"),
			wantStatus: Fail,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if c.write != "" {
				if err := os.WriteFile(path, []byte(c.write), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			check := ConfigCheck(path, func(string) error { return c.loadErr })
			r := check(context.Background(), nil)
			if r.Status != c.wantStatus {
				t.Errorf("Status = %v, want %v (Detail: %q)", r.Status, c.wantStatus, r.Detail)
			}
		})
	}
}

// ─── ThemesCheck ─────────────────────────────────────────────────────────────

// TestThemesCheck: missing dir → Skip, validate error → Warn, broken themes → Warn, clean → OK.
func TestThemesCheck(t *testing.T) {
	cases := []struct {
		name       string
		createDir  bool
		validateFn func(string) ([]string, error)
		wantStatus Status
		wantSubstr string
	}{
		{
			name:       "missing dir → Skip",
			createDir:  false,
			validateFn: func(string) ([]string, error) { return nil, nil },
			wantStatus: Skip,
		},
		{
			name:      "validate error → Warn",
			createDir: true,
			validateFn: func(string) ([]string, error) {
				return nil, errors.New("stat failed")
			},
			wantStatus: Warn,
		},
		{
			name:      "broken themes → Warn with count",
			createDir: true,
			validateFn: func(string) ([]string, error) {
				return []string{"bad1.yaml", "bad2.yaml"}, nil
			},
			wantStatus: Warn,
			wantSubstr: "2",
		},
		{
			name:       "all themes valid → OK",
			createDir:  true,
			validateFn: func(string) ([]string, error) { return nil, nil },
			wantStatus: OK,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			themesDir := filepath.Join(dir, "themes")
			if c.createDir {
				if err := os.MkdirAll(themesDir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			check := ThemesCheck(themesDir, c.validateFn)
			r := check(context.Background(), nil)
			if r.Status != c.wantStatus {
				t.Errorf("Status = %v, want %v (Detail: %q)", r.Status, c.wantStatus, r.Detail)
			}
			if c.wantSubstr != "" && !strings.Contains(r.Detail, c.wantSubstr) {
				t.Errorf("Detail %q does not contain %q", r.Detail, c.wantSubstr)
			}
		})
	}
}

// ─── ghosttyRunningCheck ─────────────────────────────────────────────────────

// TestGhosttyRunningCheck: prior installed=Fail → Skip; not-running error → Warn;
// unknown error → Fail; responsive → OK.
func TestGhosttyRunningCheck(t *testing.T) {
	cases := []struct {
		name        string
		prior       []Result
		versionJSON string
		runErr      error
		wantStatus  Status
	}{
		{
			name:       "ghostty installed check failed → Skip",
			prior:      []Result{{Name: "ghostty installed", Status: Fail}},
			wantStatus: Skip,
		},
		{
			name:       "Ghostty not running → Warn",
			runErr:     errors.New("Ghostty isn't running"),
			wantStatus: Warn,
		},
		{
			name:       "unknown error → Fail",
			runErr:     errors.New("unexpected osascript failure"),
			wantStatus: Fail,
		},
		{
			name:        "Ghostty responsive → OK",
			versionJSON: `{"version":"1.3.5"}`,
			wantStatus:  OK,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := newVersionClient(c.versionJSON, c.runErr)
			check := ghosttyRunningCheck(client)
			r := check(context.Background(), c.prior)
			if r.Status != c.wantStatus {
				t.Errorf("Status = %v, want %v (Detail: %q)", r.Status, c.wantStatus, r.Detail)
			}
		})
	}
}

// ─── ghosttyAutomationCheck ──────────────────────────────────────────────────

// TestGhosttyAutomationCheck: running check not OK → Skip; probe OK → OK;
// -1743 error → Fail; other probe error → Warn.
func TestGhosttyAutomationCheck(t *testing.T) {
	cases := []struct {
		name       string
		prior      []Result
		probeJSON  string
		probeErr   error
		wantStatus Status
	}{
		{
			name:       "running check absent → Skip",
			prior:      nil,
			wantStatus: Skip,
		},
		{
			name:       "running check = Warn → Skip",
			prior:      priorWithRunning(Warn),
			wantStatus: Skip,
		},
		{
			name:       "running check = Fail → Skip",
			prior:      priorWithRunning(Fail),
			wantStatus: Skip,
		},
		{
			name:       "probe succeeds → OK",
			prior:      priorWithRunning(OK),
			probeJSON:  `{"ok":true}`,
			wantStatus: OK,
		},
		{
			name:       "automation denied (-1743) → Fail",
			prior:      priorWithRunning(OK),
			probeErr:   errors.New("error -1743"),
			wantStatus: Fail,
		},
		{
			name:       "unknown probe error → Warn",
			prior:      priorWithRunning(OK),
			probeErr:   errors.New("unexpected failure"),
			wantStatus: Warn,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
				if c.probeErr != nil {
					return nil, nil, c.probeErr
				}
				return []byte(c.probeJSON), nil, nil
			})
			client := ghostty.New(fake)
			check := ghosttyAutomationCheck(client)
			r := check(context.Background(), c.prior)
			if r.Status != c.wantStatus {
				t.Errorf("Status = %v, want %v (Detail: %q)", r.Status, c.wantStatus, r.Detail)
			}
		})
	}
}

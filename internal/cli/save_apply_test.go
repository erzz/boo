package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/erzz/boo/internal/layout"
)

// applySaveOutcome is the user-facing decision matrix lifted out of the
// save command's RunE. These tests pin down what each outcome should do:
// where the diff goes, whether the prompt fires, and how --force interacts
// with each branch. Without them, a future refactor could (e.g.) silently
// route the lossy diff to stdout, masking unrecoverable data loss in
// scripts that only capture stderr.

// silentLossy is just enough of a SaveDiff to look like a lossy save when
// applySaveOutcome inspects Outcome and renders. We don't need a real
// before/after — only the renderer cares about the structure, and we
// assert against the renderer's output via substring matching.
func silentDiff() SaveDiff      { return SaveDiff{Outcome: OutcomeSilent} }
func structuralDiff() SaveDiff  { return SaveDiff{Outcome: OutcomeStructural, ChangedTabs: oneTabDiff()} }
func lossyDiff() SaveDiff {
	return SaveDiff{
		Outcome:     OutcomeLossy,
		ChangedTabs: oneTabDiff(),
		LossReasons: []string{`tab 0 split 1: command "x" will be lost`},
	}
}

func oneTabDiff() []TabDiff {
	return []TabDiff{{
		Index: 0,
		Name:  "main",
		Prev:  &layout.Tab{Name: "main", Splits: []layout.Split{{Cwd: "."}}},
		Next:  &layout.Tab{Name: "main", Splits: []layout.Split{{Cwd: "."}, {Direction: layout.DirRight, Cwd: "."}}},
	}}
}

func TestApplySaveOutcome_SilentProceedsWithNoOutputAndNoPrompt(t *testing.T) {
	// Idempotent saves must not nag. No diff, no prompt, no anything —
	// just "yes, write the file."
	var out, errOut bytes.Buffer
	in := strings.NewReader("") // would EOF if a prompt fires
	proceed, err := applySaveOutcome(silentDiff(), false, in, &out, &errOut)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !proceed {
		t.Fatalf("proceed = false, want true (silent should always proceed)")
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("expected no output, got stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestApplySaveOutcome_StructuralRendersToStdoutAndPrompts(t *testing.T) {
	// Structural changes are user-driven (added/removed tabs/splits) so the
	// diff is normal stdout output and the prompt is plain "Apply this change?".
	// The limitation explainer ("Why this happens" / "Recommended") fires
	// here too — even a "clean" structural diff may be hiding a flattened
	// split tree the user can't otherwise tell us about.
	var out, errOut bytes.Buffer
	in := strings.NewReader("y\n")
	proceed, err := applySaveOutcome(structuralDiff(), false, in, &out, &errOut)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !proceed {
		t.Fatalf("proceed = false, want true (user said y)")
	}
	if errOut.Len() != 0 {
		t.Errorf("structural diff should not touch stderr, got %q", errOut.String())
	}
	if !strings.Contains(out.String(), "Apply this change?") {
		t.Errorf("expected structural prompt in stdout, got %q", out.String())
	}
	// Diff content (tab name) should be in stdout.
	if !strings.Contains(out.String(), "main") {
		t.Errorf("expected diff content in stdout, got %q", out.String())
	}
	// Limitation explainer must fire on structural too.
	if !strings.Contains(out.String(), "Recommended:") {
		t.Errorf("expected 'Recommended:' explainer on structural diff, got %q", out.String())
	}
}

func TestApplySaveOutcome_StructuralDeclineReturnsFalse(t *testing.T) {
	// Empty answer (or anything not starting with y) is "no" per confirm's
	// contract. The caller is responsible for printing "aborted" — we just
	// report the decision.
	var out, errOut bytes.Buffer
	in := strings.NewReader("\n")
	proceed, err := applySaveOutcome(structuralDiff(), false, in, &out, &errOut)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if proceed {
		t.Fatalf("proceed = true, want false (user declined)")
	}
}

func TestApplySaveOutcome_LossyRendersToStderr(t *testing.T) {
	// Critical contract: the lossy diff goes to STDERR, not stdout.
	// Scripts capturing stdout must still see the loss in their stderr
	// log — otherwise --force silently destroys data.
	var out, errOut bytes.Buffer
	in := strings.NewReader("y\n")
	proceed, err := applySaveOutcome(lossyDiff(), false, in, &out, &errOut)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !proceed {
		t.Fatalf("proceed = false, want true")
	}
	if !strings.Contains(errOut.String(), "main") {
		t.Errorf("expected lossy diff content in stderr, got %q", errOut.String())
	}
	if strings.Contains(out.String(), "main") {
		t.Errorf("lossy diff content leaked to stdout: %q", out.String())
	}
	// The "Recommended: hand-author" explainer is part of the rendered
	// diff, also on stderr — verify it actually fired so a future
	// refactor can't silently drop it.
	if !strings.Contains(errOut.String(), "Recommended:") {
		t.Errorf("expected 'Recommended:' explainer in stderr, got %q", errOut.String())
	}
	// Prompt itself goes to stdout (it's interactive UI, not diagnostic).
	if !strings.Contains(out.String(), "Save anyway") {
		t.Errorf("expected lossy prompt in stdout, got %q", out.String())
	}
}

func TestApplySaveOutcome_ForceSkipsPromptButStillRendersLossyToStderr(t *testing.T) {
	// --force is the audit-trail case: no prompt, but the lossy diff must
	// still hit stderr so CI logs / `boo save -f 2>>audit.log` capture
	// what was destroyed. An empty stdin proves no prompt fired (it would
	// EOF and the answer would still be "no" — but we'd see the prompt
	// text in stdout, which we assert against).
	var out, errOut bytes.Buffer
	in := strings.NewReader("") // would EOF if anything tried to read
	proceed, err := applySaveOutcome(lossyDiff(), true, in, &out, &errOut)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !proceed {
		t.Fatalf("proceed = false, want true (--force)")
	}
	if errOut.Len() == 0 {
		t.Errorf("--force must still print lossy diff to stderr for audit, got empty")
	}
	if strings.Contains(out.String(), "Save anyway") {
		t.Errorf("--force should skip the prompt, but prompt text appeared in stdout: %q", out.String())
	}
}

func TestApplySaveOutcome_ForceSkipsStructuralPrompt(t *testing.T) {
	// Same shape as the lossy --force test, but for structural.
	// Structural diff still goes to stdout under --force (it's the normal
	// place for non-error output).
	var out, errOut bytes.Buffer
	in := strings.NewReader("")
	proceed, err := applySaveOutcome(structuralDiff(), true, in, &out, &errOut)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !proceed {
		t.Fatalf("proceed = false, want true (--force)")
	}
	if strings.Contains(out.String(), "Apply this change?") {
		t.Errorf("--force should skip prompt, got %q", out.String())
	}
}

func TestApplySaveOutcome_UnknownOutcomeBlocksSave(t *testing.T) {
	// If a future SaveOutcome value is added but applySaveOutcome's switch
	// isn't updated, we must NOT silently proceed (that would write a
	// layout the user never confirmed). Refusing to save with a clear
	// error mirrors the rest of save.go's "refuse to overwrite when
	// uncertain" stance.
	var out, errOut bytes.Buffer
	in := strings.NewReader("")
	proceed, err := applySaveOutcome(SaveDiff{Outcome: SaveOutcome(99)}, false, in, &out, &errOut)
	if err == nil {
		t.Fatalf("expected error for unknown outcome, got nil")
	}
	if proceed {
		t.Fatalf("unknown outcome must not proceed")
	}
}

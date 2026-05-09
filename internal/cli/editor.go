package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// buildEditorCmd constructs an *exec.Cmd that will open path in the
// user's preferred editor.
//
// Resolution order:
//  1. editorOverride — non-empty string passed by the caller (e.g. a
//     --editor flag value, or a pre-resolved $EDITOR string).
//  2. $EDITOR environment variable.
//  3. $VISUAL environment variable.
//  4. Error with an actionable message.
//
// Multi-word editor values (e.g. "code --wait", "emacs -nw -Q") are
// tokenised with a minimal shell-split that handles unquoted tokens,
// single-quoted groups, and double-quoted groups. Backtick substitution
// and subshells are rejected.
//
// This function intentionally bypasses internal/exec.Runner. Interactive
// editors need direct access to the controlling TTY (stdin/stdout/stderr
// pass-through), which the Runner's capture-buffer model cannot support.
// This is a documented exception — see also fzf.go (runFzf) and the
// tea.ExecProcess call in picker_helpers.go (onOpenLayout).
func buildEditorCmd(editorOverride, path string) (*exec.Cmd, error) {
	editor := editorOverride
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return nil, fmt.Errorf("set $EDITOR (or $VISUAL) to your editor of choice, e.g. `nvim` or `code --wait`")
	}
	name, args, err := splitEditorCommand(editor)
	if err != nil {
		return nil, fmt.Errorf("parse editor command %q: %w", editor, err)
	}
	args = append(args, path)
	return exec.Command(name, args...), nil //nolint:gosec // args are user-supplied editor tokens, not shell strings
}

// openInEditor builds an editor command via buildEditorCmd and runs it
// with stdin/stdout/stderr attached to the process's controlling terminal.
// Intended for CLI commands (boo config edit, boo edit) that hand off
// to the editor and wait for it to exit before continuing.
func openInEditor(editorOverride, path string) error {
	cmd, err := buildEditorCmd(editorOverride, path)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// splitEditorCommand tokenises an editor command string into a command
// name and argument list. It handles:
//   - unquoted tokens delimited by ASCII whitespace
//   - single-quoted groups (no escape processing inside)
//   - double-quoted groups (no escape processing inside)
//
// Backticks are rejected because subshell substitution is not supported
// and silently expanding them would be unexpected and potentially unsafe.
// Unterminated quotes are also rejected.
func splitEditorCommand(s string) (cmd string, args []string, err error) {
	tokens, err := shsplit(s)
	if err != nil {
		return "", nil, err
	}
	if len(tokens) == 0 {
		return "", nil, fmt.Errorf("empty editor command")
	}
	return tokens[0], tokens[1:], nil
}

// shsplit is the minimal tokeniser backing splitEditorCommand. It splits
// s into whitespace-separated tokens, honouring single and double quotes.
func shsplit(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '`':
			return nil, fmt.Errorf("backtick substitution is not supported in editor command %q", s)
		case inSingle:
			if ch == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(ch)
			}
		case inDouble:
			if ch == '"' {
				inDouble = false
			} else {
				cur.WriteByte(ch)
			}
		case ch == '\'':
			inSingle = true
		case ch == '"':
			inDouble = true
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(ch)
		}
	}

	if inSingle {
		return nil, fmt.Errorf("unterminated single quote in editor command %q", s)
	}
	if inDouble {
		return nil, fmt.Errorf("unterminated double quote in editor command %q", s)
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}

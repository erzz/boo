package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// buildEditorCmd constructs an *exec.Cmd that opens path in the user's preferred editor.
//
// Resolution order: editorOverride → $EDITOR → $VISUAL → error.
// Multi-word values (e.g. "code --wait") are tokenised; backtick substitution and
// unterminated quotes are rejected.
//
// Intentionally bypasses internal/exec.Runner — interactive editors need direct TTY
// access that the Runner's capture-buffer model cannot support. Documented exception;
// see also runFzf (picker_helpers.go) and tea.ExecProcess (picker_helpers.go onOpenLayout).
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

// openInEditor builds an editor command and runs it with TTY attached.
// Intended for CLI commands (boo config edit, boo edit) that hand off to the editor.
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

// splitEditorCommand tokenises an editor command into name and args.
// Handles unquoted tokens, single-quoted groups, and double-quoted groups.
// Backticks and unterminated quotes are rejected.
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

// shsplit is the minimal tokeniser backing splitEditorCommand.
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

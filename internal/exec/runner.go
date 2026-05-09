// Package exec provides a Runner abstraction over os/exec so that code which
// shells out can be unit-tested with a fake. Production code MUST go through
// Runner — see AGENTS.md for documented exceptions.
package exec

import (
	"context"
	"os/exec"
)

// Runner runs an external command and returns its stdout, stderr, and any
// error. Implementations are expected to be safe for concurrent use.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
	RunWithStdin(ctx context.Context, stdin []byte, name string, args ...string) (stdout, stderr []byte, err error)
}

// Real is the production Runner backed by os/exec.
type Real struct{}

// NewReal returns a Runner that executes commands via os/exec.
func NewReal() Runner { return Real{} }

// Run executes name with args and captures stdout/stderr.
func (Real) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	return runCmd(ctx, nil, name, args...)
}

// RunWithStdin executes name with args, piping stdin into the process.
func (Real) RunWithStdin(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, []byte, error) {
	return runCmd(ctx, stdin, name, args...)
}

func runCmd(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = bytesReader(stdin)
	}
	var outBuf, errBuf bytesBuffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

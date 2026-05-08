package exec

import (
	"context"
	"fmt"
	"sync"
)

// Fake is a Runner for tests. It records calls and returns canned responses.
// Safe for concurrent use.
type Fake struct {
	mu       sync.Mutex
	Calls    []Call
	Response func(name string, args []string, stdin []byte) (stdout, stderr []byte, err error)
}

// Call is a recorded invocation.
type Call struct {
	Name  string
	Args  []string
	Stdin []byte
}

// NewFake creates a Fake. If respond is nil, all calls succeed with empty output.
func NewFake(respond func(name string, args []string, stdin []byte) ([]byte, []byte, error)) *Fake {
	return &Fake{Response: respond}
}

// Run records the call and returns the canned response.
func (f *Fake) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	return f.dispatch(name, args, nil)
}

// RunWithStdin records the call (including stdin) and returns the canned response.
func (f *Fake) RunWithStdin(_ context.Context, stdin []byte, name string, args ...string) ([]byte, []byte, error) {
	return f.dispatch(name, args, stdin)
}

func (f *Fake) dispatch(name string, args []string, stdin []byte) ([]byte, []byte, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, Call{Name: name, Args: append([]string(nil), args...), Stdin: append([]byte(nil), stdin...)})
	respond := f.Response
	f.mu.Unlock()
	if respond == nil {
		return nil, nil, nil
	}
	return respond(name, args, stdin)
}

// LastCall returns the most recent recorded call, or an error if none.
func (f *Fake) LastCall() (Call, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.Calls) == 0 {
		return Call{}, fmt.Errorf("no calls recorded")
	}
	return f.Calls[len(f.Calls)-1], nil
}

// Snapshot returns a copy of all recorded calls.
func (f *Fake) Snapshot() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Call, len(f.Calls))
	copy(out, f.Calls)
	return out
}

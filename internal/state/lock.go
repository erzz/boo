package state

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Lock is an advisory cross-process file lock via flock(2). Hold until Close.
type Lock struct {
	f    *os.File
	path string
}

// Acquire blocks until the lock at path is held exclusively by this process.
// The parent directory must already exist.
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("state: lock parent: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("state: open lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("state: flock %s: %w", path, err)
	}
	return &Lock{f: f, path: path}, nil
}

// Close releases the lock and closes the underlying file.
func (l *Lock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	// flock is released when the fd is closed.
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}

// LockPath returns the canonical lock file path inside ConfigDir.
func (p Paths) LockPath() string { return filepath.Join(p.ConfigDir, ".lock") }

// WithLock acquires the boo data lock, runs fn, and releases the lock.
// Any error from fn is returned unchanged.
func (p Paths) WithLock(fn func() error) error {
	l, err := Acquire(p.LockPath())
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()
	return fn()
}

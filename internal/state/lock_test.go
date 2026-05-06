package state

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestWithLock_SerializesConcurrentCallers(t *testing.T) {
	root := t.TempDir()
	p := ForRoot(root)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	var inFlight, maxInFlight int32
	var calls int32
	const n = 8
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			err := p.WithLock(func() error {
				cur := atomic.AddInt32(&inFlight, 1)
				for {
					m := atomic.LoadInt32(&maxInFlight)
					if cur <= m || atomic.CompareAndSwapInt32(&maxInFlight, m, cur) {
						break
					}
				}
				atomic.AddInt32(&calls, 1)
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&inFlight, -1)
				return nil
			})
			if err != nil {
				t.Errorf("WithLock: %v", err)
			}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
	if got := atomic.LoadInt32(&calls); got != n {
		t.Fatalf("expected %d calls, got %d", n, got)
	}
	// flock is per-fd so within a single process goroutines actually serialize:
	if got := atomic.LoadInt32(&maxInFlight); got != 1 {
		t.Fatalf("expected max inFlight 1 under lock, got %d", got)
	}
}

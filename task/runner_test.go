package task

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerLifecycle(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	canceled := make(chan struct{})
	r, err := NewRunner(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.Done():
	default:
		t.Fatal("idle Done not ready")
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := r.Start(context.Background()); err != ErrRunning {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() { r.Stop(); close(stopped) }()
	<-canceled
	select {
	case <-stopped:
		t.Fatal("Stop did not wait for exit")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop blocked")
	}
	if r.Running() || !errors.Is(r.Err(), context.Canceled) {
		t.Fatal("exit state", r.Err())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Start(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := NewRunner(nil); err == nil {
		t.Fatal("nil handler")
	}
}
func TestRunnerImmediateStop(t *testing.T) {
	r, _ := NewRunner(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	for range 100 {
		if err := r.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		r.Stop()
	}
	want := errors.New("failed")
	f, _ := NewRunner(func(context.Context) error { return want })
	if err := f.Run(context.Background()); err != want || f.Err() != want {
		t.Fatal("error lost")
	}
}
func TestOneRunReplacement(t *testing.T) {
	var active, overlap atomic.Int32
	entered := make(chan struct{}, 10)
	r, err := NewOneRun(func(ctx context.Context) error {
		if active.Add(1) != 1 {
			overlap.Store(1)
		}
		defer active.Add(-1)
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); r.Run(context.Background()) }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("replacement blocked")
		}
	}
	r.Stop()
	wg.Wait()
	if overlap.Load() != 0 || active.Load() != 0 || r.Running() {
		t.Fatal("overlap or unfinished run")
	}
	if _, err := NewOneRun(nil); err == nil {
		t.Fatal("nil handler")
	}
}

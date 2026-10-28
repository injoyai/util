package safe_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/injoyai/util/safe"
)

func TestCloserLifecycle(t *testing.T) {
	c := safe.NewCloser()
	oldDone := c.Done()
	reason := errors.New("disconnected")
	callbackErr := errors.New("resource close failed")
	calls := 0
	c.SetCloseFunc(func(err error) error {
		calls++
		if !c.Closed() || !errors.Is(c.Err(), err) {
			t.Fatal("callback cannot observe close state")
		}
		select {
		case <-c.Done():
		default:
			t.Fatal("callback ran before close notification")
		}
		if err := c.Close(); err != nil {
			t.Fatal("reentrant close", err)
		}
		return callbackErr
	})
	if c.Closed() || c.Err() != nil {
		t.Fatal("new closer starts closed")
	}
	if err := c.CloseWithErr(nil); err != nil || c.Closed() {
		t.Fatal("nil reason must leave closer open", err)
	}
	if err := c.CloseWithErr(reason); !errors.Is(err, callbackErr) {
		t.Fatal("callback error lost", err)
	}
	if !errors.Is(c.Err(), reason) || calls != 1 {
		t.Fatal("close reason or callback count", c.Err(), calls)
	}
	if err := c.CloseWithErr(errors.New("later reason")); err != nil || !errors.Is(c.Err(), reason) || calls != 1 {
		t.Fatal("duplicate close changed state", err)
	}
	c.Reset()
	if c.Closed() || c.Err() != nil || c.Done() == oldDone {
		t.Fatal("reset did not start a new lifecycle")
	}
	select {
	case <-oldDone:
	default:
		t.Fatal("old notification reopened")
	}
	select {
	case <-c.Done():
		t.Fatal("new notification already closed")
	default:
	}
	if err := c.Close(); !errors.Is(err, callbackErr) || calls != 2 || c.Err() == nil {
		t.Fatal("callback not preserved across reset", err, calls)
	}
}

func TestCloserConcurrentClose(t *testing.T) {
	c := safe.NewCloser()
	reason := errors.New("disconnected")
	var calls atomic.Int32
	c.SetCloseFunc(func(error) error { calls.Add(1); return nil })
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.CloseWithErr(reason)
			if !c.Closed() || !errors.Is(c.Err(), reason) {
				t.Error("inconsistent close state")
			}
			<-c.Done()
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate close callback", calls.Load())
	}
}

func TestCloserResetOpen(t *testing.T) {
	c := safe.NewCloser()
	oldDone := c.Done()
	calls := 0
	c.SetCloseFunc(func(error) error { calls++; return nil })
	c.Reset()
	select {
	case <-oldDone:
	default:
		t.Fatal("reset left old waiters blocked")
	}
	if c.Closed() || c.Err() != nil || calls != 0 {
		t.Fatal("reset closed resources or retained old state")
	}
	c.SetCloseFunc(nil)
	if err := c.Close(); err != nil || calls != 0 {
		t.Fatal("removed callback executed", err)
	}
}

func TestNewCloserErr(t *testing.T) {
	for _, reason := range []error{nil, errors.New("already closed")} {
		c := safe.NewCloserErr(reason)
		if !c.Closed() || c.Err() != reason {
			t.Fatal("closed state depends on error", c.Err())
		}
		select {
		case <-c.Done():
		default:
			t.Fatal("closed constructor left Done open")
		}
		c.SetCloseFunc(func(error) error { t.Error("late callback executed"); return nil })
		c.Close()
	}
	var c *safe.Closer
	if !c.Closed() {
		t.Fatal("nil closer must report closed")
	}
}

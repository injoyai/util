package wait

import (
	"errors"
	"runtime"
	"testing"
	"time"
)

func waitRegistered[K comparable, V any](t *testing.T, x *Generic[K, V], key K) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !x.IsWait(key) {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered")
		}
		runtime.Gosched()
	}
}

func TestNew2(t *testing.T) {
	x := NewGeneric[string, int](time.Second)
	for i := 0; i < 3; i++ {
		result := make(chan int, 1)
		go func() { v, _ := x.Wait("key"); result <- v }()
		waitRegistered(t, x, "key")
		if !x.Done("key", i) || <-result != i {
			t.Fatal("incorrect response")
		}
	}
}

func TestNew(t *testing.T) {
	x := NewGeneric[string, int](time.Second).SetReuse(true)
	result := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() { v, _ := x.Wait("key"); result <- v }()
	}
	deadline := time.Now().Add(time.Second)
	for {
		x.mu.Lock()
		p := x.m["key"]
		ready := p != nil && len(p.waiters) == 2
		x.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waiters not registered")
		}
		runtime.Gosched()
	}
	x.Done("key", 7)
	if <-result != 7 || <-result != 7 {
		t.Fatal("response not shared")
	}
	if x.IsWait("key") {
		t.Fatal("completed waiters retained")
	}
}

func TestErr(t *testing.T) {
	x := NewGeneric[string, int](time.Second)
	expected := errors.New("response failed")
	result := make(chan error, 1)
	go func() { _, err := x.Wait("key"); result <- err }()
	waitRegistered(t, x, "key")
	x.Done("key", 0, expected)
	if err := <-result; !errors.Is(err, expected) {
		t.Fatal(err)
	}
}

func TestAsyncLifetime(t *testing.T) {
	x := NewGeneric[string, int](time.Second)
	count := 0
	x.Async("finite", func(int, error) { count++ }, 2)
	if !x.Done("finite", 1) || !x.Done("finite", 2) || x.Done("finite", 3) || count != 2 {
		t.Fatal("finite callback lifetime", count)
	}
	x.Async("persistent", func(int, error) { count++ }, -1, 0)
	for i := 0; i < 3; i++ {
		if !x.Done("persistent", i) {
			t.Fatal("persistent callback removed")
		}
	}
	if count != 5 {
		t.Fatal("persistent count", count)
	}
	x.Async("expired", func(int, error) { t.Error("expired callback ran") }, 1, time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for x.IsWait("expired") {
		if time.Now().After(deadline) {
			t.Fatal("expired callback retained")
		}
		runtime.Gosched()
	}
	if x.Done("expired", 1) {
		t.Fatal("expired key retained")
	}
}

func TestAsyncReentrant(t *testing.T) {
	x := NewGeneric[int, int](time.Second)
	calls := 0
	x.Async(1, func(int, error) {
		calls++
		x.Async(1, func(int, error) { calls++ }, 1)
		x.Done(1, 2)
	}, 1)
	x.Done(1, 1)
	if calls != 2 || x.IsWait(1) {
		t.Fatal("reentrant callback", calls)
	}
}

func TestSyncTimeoutCleanup(t *testing.T) {
	x := NewGeneric[int, int](time.Second)
	for _, timeout := range []time.Duration{0, time.Millisecond} {
		if _, err := x.Sync(1, timeout); err == nil || x.IsWait(1) {
			t.Fatal("timed-out waiter retained", err)
		}
	}
}

func TestQueuedResponsesInOrder(t *testing.T) {
	x := NewGeneric[int, int](time.Second)
	first, second := make(chan int, 1), make(chan int, 1)
	go func() { v, _ := x.Sync(1); first <- v }()
	waitRegistered(t, x, 1)
	go func() { v, _ := x.Sync(1); second <- v }()
	deadline := time.Now().Add(time.Second)
	for {
		x.mu.Lock()
		p := x.m[1]
		ready := p != nil && len(p.waiters) == 2
		x.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second waiter missing")
		}
		runtime.Gosched()
	}
	x.Done(1, 7)
	if <-first != 7 {
		t.Fatal("first response out of order")
	}
	x.Done(1, 9)
	if <-second != 9 || x.IsWait(1) {
		t.Fatal("second response lost")
	}
}

func TestCloseReleasesAndClears(t *testing.T) {
	x := NewGeneric[int, int](time.Hour)
	result := make(chan error, 1)
	go func() { _, err := x.Sync(1); result <- err }()
	waitRegistered(t, x, 1)
	x.Async(2, func(int, error) { t.Error("pending callback executed by Close") }, -1, time.Hour)
	x.Close()
	select {
	case err := <-result:
		if err != ErrClosed {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter retained")
	}
	if _, err := x.Sync(1); err != ErrClosed {
		t.Fatal(err)
	}
	x.Async(3, func(int, error) {}, 1)
	if x.Done(1, 7) || x.IsWait(2) || x.IsWait(3) {
		t.Fatal("closed waiter retained work")
	}
	x.Close()
}

package timeout

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	x := New[int](time.Millisecond, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	expired := make(chan int, 1)
	x.SetDealFunc(func(key int) error { expired <- key; cancel(); return nil })
	x.Keep(1)
	done := make(chan struct{})
	go func() { x.Run(ctx); close(done) }()
	select {
	case key := <-expired:
		if key != 1 {
			t.Fatal(key)
		}
	case <-time.After(time.Second):
		t.Fatal("not expired")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run ignored cancellation")
	}
}

func TestHeartbeatDuringCallback(t *testing.T) {
	x := New[int](time.Nanosecond, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	x.SetDealFunc(func(key int) error { x.Keep(key); cancel(); return nil })
	x.Keep(1)
	x.Run(ctx)
	x.mu.Lock()
	_, exists := x.m[1]
	x.mu.Unlock()
	if !exists {
		t.Fatal("fresh heartbeat deleted")
	}
}

func TestStaleTimeoutSnapshotIsRechecked(t *testing.T) {
	for _, change := range []string{"heartbeat", "delete", "timeout"} {
		t.Run(change, func(t *testing.T) {
			x := New[int](time.Hour, time.Millisecond)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			x.SetDealFunc(func(int) error {
				if calls.Add(1) == 1 {
					close(entered)
					<-release
				} else {
					cancel()
				}
				return nil
			})
			// 两个 key 同时过期，首个回调暂停时刷新第二个 key。
			x.m[1], x.m[2] = time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)
			go func() { x.Run(ctx); close(done) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("timeout callback did not start")
			}
			switch change {
			case "heartbeat":
				x.Keep(1)
				x.Keep(2)
			case "delete":
				x.Del(1)
				x.Del(2)
			case "timeout":
				x.SetTimeout(24 * time.Hour)
			}
			close(release)
			select {
			case <-done:
			case <-time.After(30 * time.Millisecond):
				cancel()
				<-done
			}
			if calls.Load() != 1 {
				t.Fatal("stale snapshot triggered another callback", calls.Load())
			}
		})
	}
}

package chans

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(time.Second):
		t.Fatal("operation blocked")
		var zero T
		return zero
	}
}
func TestSafeCloseUnblocksSend(t *testing.T) {
	for _, capacity := range []int{0, 1} {
		s, err := NewSafe[int](capacity)
		if err != nil {
			t.Fatal(err)
		}
		if capacity > 0 && !s.Add(1) {
			t.Fatal("buffered send")
		}
		done := make(chan bool, 1)
		go func() { done <- s.Add(2) }()
		s.Close()
		if receive(t, done) || s.Add(3) || s.Try(4) {
			t.Fatal("send after close")
		}
		if capacity > 0 && receive(t, s.Chan) != 1 {
			t.Fatal("buffer lost")
		}
		if _, ok := <-s.Chan; ok {
			t.Fatal("channel not closed")
		}
		s.Close()
	}
	if _, err := NewSafe[int](-1); err == nil {
		t.Fatal("negative capacity")
	}
	s, _ := NewSafe[int](0)
	defer s.Close()
	if s.Try(1) || s.Timeout(1, time.Millisecond) {
		t.Fatal("unexpected send")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Send(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSubscribeConcurrentClose(t *testing.T) {
	s := NewSubscribe[string, int]()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range 100 {
				sub, err := s.Subscribe("x", 1)
				if err != nil {
					t.Error(err)
					return
				}
				s.Publish("x", n, 0)
				sub.Close()
			}
		}()
	}
	wg.Add(1)
	go func() { defer wg.Done(); s.Last(); s.Close() }()
	wg.Wait()
	sub, err := s.Subscribe("x", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := <-sub.Chan; ok {
		t.Fatal("subscription after close")
	}
	s.mu.RLock()
	n := len(s.m)
	s.mu.RUnlock()
	if n != 0 {
		t.Fatal("closed subscriptions retained")
	}
}
func TestSubscribeDeliveryAndBlockedClose(t *testing.T) {
	s := NewSubscribe[string, int]()
	defer s.Close()
	a, _ := s.Subscribe("x", 1)
	b, _ := s.Subscribe("x", 1)
	s.Publish("x", 7, 0)
	if receive(t, a.Chan) != 7 || receive(t, b.Chan) != 7 || s.Last() != 7 {
		t.Fatal("broadcast")
	}
	a.Close()
	b.Close()
	c, _ := s.Subscribe("x", 0)
	done := make(chan struct{})
	go func() { s.Publish("x", 9, -1); close(done) }()
	c.Close()
	receive(t, done)
}

func TestIOPartialReadAndClose(t *testing.T) {
	x, err := NewIO(2)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte("abc")
	if _, err = x.Write(input); err != nil {
		t.Fatal(err)
	}
	input[0] = 'z'
	x.Write([]byte("def"))
	x.Close()
	p := make([]byte, 1)
	if n, err := x.Read(p); n != 1 || err != nil || p[0] != 'a' {
		t.Fatal("partial read", n, err)
	}
	if rest, err := x.ReadBytes(); err != nil || string(rest) != "bc" {
		t.Fatal("lost remainder", rest, err)
	}
	if rest, err := io.ReadAll(x); err != nil || string(rest) != "def" {
		t.Fatal("drain", rest, err)
	}
	if _, err := x.Write([]byte("x")); err != io.ErrClosedPipe {
		t.Fatal(err)
	}
	blocked, _ := NewIO(0)
	done := make(chan error, 1)
	go func() { _, err := blocked.Write([]byte("x")); done <- err }()
	blocked.Close()
	if err := receive(t, done); err != io.ErrClosedPipe {
		t.Fatal(err)
	}
	if _, err := NewIO(-1); err == nil {
		t.Fatal("negative capacity")
	}
	empty, _ := NewIO(1)
	defer empty.Close()
	if n, err := empty.Read(nil); n != 0 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := empty.Write(nil); n != 0 || err != nil || empty.Len() != 0 {
		t.Fatal(n, err)
	}
}

func TestIOWaitsForInflightWriteBeforeEOF(t *testing.T) {
	x, err := NewIO(1)
	if err != nil {
		t.Fatal(err)
	}
	// 暂停在途写入，直到读端已观察到关闭信号。
	x.writeMu.RLock()
	pending := true
	closed := make(chan struct{})
	go func() { x.Close(); close(closed) }()
	t.Cleanup(func() {
		if pending {
			x.writeMu.RUnlock()
		}
		receive(t, closed)
	})
	receive(t, x.closeSign)
	type result struct {
		data []byte
		err  error
	}
	read := make(chan result, 1)
	go func() { data, err := x.ReadBytes(); read <- result{data, err} }()
	select {
	case got := <-read:
		t.Fatalf("read returned before pending write settled: %q, %v", got.data, got.err)
	case <-time.After(10 * time.Millisecond):
	}
	x.ch <- []byte("last message")
	x.writeMu.RUnlock()
	pending = false
	got := receive(t, read)
	if got.err != nil || string(got.data) != "last message" {
		t.Fatalf("pending write lost: %q, %v", got.data, got.err)
	}
	if _, err := x.ReadBytes(); err != io.EOF {
		t.Fatal("expected EOF after draining", err)
	}
}

func TestQueueCover(t *testing.T) {
	q, err := NewQueueCover[int](2)
	if err != nil {
		t.Fatal(err)
	}
	q.Append(1)
	q.Append(2)
	q.Append(3)
	q.Close()
	for _, want := range []int{2, 3} {
		v, err := q.Pop(context.Background())
		if err != nil || v != want {
			t.Fatal(v, err)
		}
	}
	if q.Append(4) {
		t.Fatal("append after close")
	}
	if _, err := q.Pop(context.Background()); err != io.EOF {
		t.Fatal(err)
	}
	if _, err := NewQueueCover[int](0); err == nil {
		t.Fatal("zero capacity")
	}
	concurrent, _ := NewQueueCover[int](4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 100 {
				concurrent.Append(i)
			}
		}()
	}
	wg.Wait()
	if concurrent.Len() != 4 {
		t.Fatal("queue length")
	}
	concurrent.Close()
}

func TestWaitOnceBroadcast(t *testing.T) {
	w := NewWaitOnce[int]()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) { defer wg.Done(); w.Done(i) }(i)
	}
	wg.Wait()
	first, err := w.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		v, err := w.Wait(context.Background())
		if v != first || err != nil {
			t.Fatal("value not shared")
		}
	}
	pending := NewWaitOnce[int]()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pending.Wait(ctx); err != context.Canceled {
		t.Fatal(err)
	}
}

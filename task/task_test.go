package task

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryAndCancellation(t *testing.T) {
	want := errors.New("failed")
	count := 0
	if err := Retry(context.Background(), 2, nil, func(context.Context) error { count++; return want }); !errors.Is(err, want) || count != 3 {
		t.Fatal(count, err)
	}
	count = 0
	if err := Retry(context.Background(), 2, nil, func(context.Context) error {
		count++
		if count < 3 {
			return want
		}
		return nil
	}); err != nil || count != 3 {
		t.Fatal(count, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := Retry(ctx, 5, func(int) time.Duration { cancel(); return time.Hour }, func(context.Context) error { return want })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := Retry(context.Background(), -1, nil, func(context.Context) error { return nil }); err == nil {
		t.Fatal("invalid retries accepted")
	}
}

func TestMapConcurrencyAndOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{})
	var active, peak atomic.Int32
	var results []Result[int]
	var mapErr error
	go func() {
		defer close(done)
		results, mapErr = Map(ctx, []int{1, 2, 3, 4, 5}, 2, func(ctx context.Context, n int) (int, error) {
			now := active.Add(1)
			defer active.Add(-1)
			for {
				old := peak.Load()
				if old >= now || peak.CompareAndSwap(old, now) {
					break
				}
			}
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-release:
				return n * 2, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		})
	}()
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(release)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if mapErr != nil || peak.Load() != 2 {
		t.Fatal(peak.Load(), mapErr)
	}
	var values []int
	for _, r := range results {
		values = append(values, r.Value)
	}
	if !slices.Equal(values, []int{2, 4, 6, 8, 10}) {
		t.Fatal(values)
	}
}

func TestMapErrorsAndCancellation(t *testing.T) {
	want := errors.New("item failed")
	results, err := Map(context.Background(), []int{1, 2, 3}, 2, func(_ context.Context, n int) (int, error) {
		if n == 2 {
			return 0, want
		}
		return n, nil
	})
	if !errors.Is(err, want) || results[1].Err != want || results[2].Value != 3 {
		t.Fatal(results, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var count atomic.Int32
	results, err = Map(ctx, []int{1, 2}, 1, func(context.Context, int) (int, error) { count.Add(1); return 0, nil })
	if count.Load() != 0 || !errors.Is(err, context.Canceled) || results[0].Err == nil || results[1].Err == nil {
		t.Fatal(results, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	exited := make(chan struct{})
	_, err = Map(ctx, []int{1, 2}, 1, func(ctx context.Context, _ int) (int, error) {
		cancel()
		defer close(exited)
		<-ctx.Done()
		return 0, ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("Map returned before handler exited")
	}
}

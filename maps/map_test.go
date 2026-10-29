package maps_test

import (
	"github.com/injoyai/util/maps"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapAtomicOperationsAndCallbacks(t *testing.T) {
	m := maps.New[string, int]()
	defer m.Close()
	var wg sync.WaitGroup
	results := make(chan int, 100)
	var inserted int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			value, existed := m.GetOrSet("key", v)
			if !existed {
				atomic.AddInt32(&inserted, 1)
			}
			results <- value
		}(i)
	}
	wg.Wait()
	close(results)
	expected, _ := m.Get("key")
	for v := range results {
		if v != expected {
			t.Fatal("load-or-store mismatch")
		}
	}
	if inserted != 1 {
		t.Fatal("multiple insertions", inserted)
	}
	var removed int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := m.GetAndDel("key"); ok {
				atomic.AddInt32(&removed, 1)
			}
		}()
	}
	wg.Wait()
	if removed != 1 {
		t.Fatal("multiple removals", removed)
	}
	m.OnSet(func(k string, v int) {
		if k == "computed" {
			m.GetOrSetByHandler2(k, func() (int, error) { return 99, nil })
		}
	})
	if v, err := m.GetOrSetByHandler2("computed", func() (int, error) { return 7, nil }); err != nil || v != 7 {
		t.Fatal("computed", v)
	}
	m.Set("a", 1)
	m.Range(func(k string, v int) bool { m.Del(k); return true })
	if len(m.Map()) != 0 {
		t.Fatal("range callback could not delete")
	}
}
func TestMapHandlerSingleExecutionAndExpiration(t *testing.T) {
	m := maps.New[int, int]()
	defer m.Close()
	var wg sync.WaitGroup
	var calls int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.GetOrSetByHandler2(1, func() (int, error) { atomic.AddInt32(&calls, 1); return 7, nil })
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatal("duplicate factory calls", calls)
	}
	m.Set(2, 8, time.Hour)
	copy := m.Clone()
	defer copy.Close()
	m.SetExpiration(2, time.Nanosecond)
	time.Sleep(time.Millisecond)
	m.Clear()
	if m.Has(2) || !copy.Has(2) {
		t.Fatal("clone shared expiration")
	}
	m.Set(3, 9, time.Hour)
	m.SetExpiration(3, 0)
	if _, ok := m.Get(3); !ok {
		t.Fatal("permanent expiration")
	}
	var events sync.WaitGroup
	events.Add(2)
	go func() {
		defer events.Done()
		for i := 0; i < 100; i++ {
			m.OnGet(func(int, int, bool) {})
			m.OnSet(func(int, int) {})
			m.OnDel(func(int) {})
		}
	}()
	go func() {
		defer events.Done()
		for i := 0; i < 100; i++ {
			m.Set(4, i)
			m.Get(4)
			m.SetExpiration(4, time.Hour)
			m.Del(4)
		}
	}()
	events.Wait()
	sub, err := m.Subscribe(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.RunClear(time.Millisecond)
	m.Close()
	if _, ok := <-sub.Chan; ok {
		t.Fatal("map close did not stop subscription")
	}
}

func TestGetOrSetByHandler(t *testing.T) {
	m := maps.New[string, int]()
	defer m.Close()
	if got := m.GetOrSetByHandler("missing", nil); got != 0 || m.Has("missing") {
		t.Fatal("nil factory should only read", got)
	}
	m.Set("cached", 9)
	if got := m.GetOrSetByHandler("cached", nil); got != 9 {
		t.Fatal("nil factory lost cached value", got)
	}

	var calls atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := m.GetOrSetByHandler("computed", func() int {
				calls.Add(1)
				return 7
			})
			if got != 7 {
				t.Errorf("computed value = %d, want 7", got)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate factory calls", calls.Load())
	}
	m.GetOrSetByHandler("expires", func() int { return 1 }, time.Nanosecond)
	time.Sleep(time.Millisecond)
	if m.Has("expires") {
		t.Fatal("expiration not forwarded")
	}
}

func TestComputedValueSurvivesImmediateExpiration(t *testing.T) {
	for name, call := range map[string]func(*maps.Map[string, int]) (int, error){
		"value": func(m *maps.Map[string, int]) (int, error) {
			value, loaded := m.GetOrSet("x", 42, time.Nanosecond)
			if loaded {
				t.Error("new value reported as cached")
			}
			return value, nil
		},
		"handler": func(m *maps.Map[string, int]) (int, error) {
			return m.GetOrSetByHandler("x", func() int { return 42 }, time.Nanosecond), nil
		},
		"handler2": func(m *maps.Map[string, int]) (int, error) {
			return m.GetOrSetByHandler2("x", func() (int, error) { return 42, nil }, time.Nanosecond)
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := maps.New[string, int]()
			defer m.Close()
			m.OnSet(func(_ string, value int) {
				if value != 42 {
					t.Errorf("notification value=%d, want 42", value)
				}
			})
			if value, err := call(m); err != nil || value != 42 {
				t.Fatalf("value=%d err=%v, want 42,nil", value, err)
			}
		})
	}
}

func TestBitConcurrentGroups(t *testing.T) {
	var b maps.Bit
	var wg sync.WaitGroup
	for n := uint64(0); n < 64; n++ {
		wg.Add(1)
		go func(n uint64) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				b.Set(n, true)
				b.Del(n)
			}
			b.Set(n, true)
		}(n)
	}
	wg.Wait()
	for n := uint64(0); n < 64; n++ {
		if !b.Get(n) {
			t.Fatalf("lost bit %d", n)
		}
	}
	b.Del(63)
	if b.Get(63) || !b.Get(62) {
		t.Fatal("clear affected neighbor")
	}
}

func TestMapLifecycle(t *testing.T) {
	m := maps.New[string, int]()
	m.Set("x", 7)
	m.RunClear(time.Millisecond)
	m.Close()
	sub, err := m.Subscribe("x", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := <-sub.Chan; ok {
		t.Fatal("subscription after close")
	}
	if v, ok := m.Get("x"); !ok || v != 7 {
		t.Fatal("Close cleared stored values")
	}
	m.RunClear(time.Millisecond) // 关闭后不会重启清理协程。
	if _, err := m.Subscribe("x", -1); err == nil {
		t.Fatal("invalid capacity accepted")
	}
}

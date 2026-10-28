package keep

import (
	"sync"
	"testing"
	"time"
)

func TestHeartbeatHistory(t *testing.T) {
	k := New[string](time.Second)
	if k.Keeping("x") {
		t.Fatal("missing heartbeat")
	}
	k.Keep("x")
	if !k.Keeping("x") || k.Keeping("x", 2) {
		t.Fatal("missing history accepted")
	}
	k.Keep("x")
	if !k.Keeping("x", 2) || k.Keeping("x", 0) || k.Keeping("x", 7) {
		t.Fatal("invalid history count")
	}
	k.OnKeep(func(key string, _ time.Time) { k.Keeping(key); k.SetTimeout(time.Hour) })
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				k.Keep("x")
				k.Keeping("x", 2)
			}
		}()
	}
	wg.Wait()
	k.Del("x")
	if k.Keeping("x") {
		t.Fatal("deleted heartbeat")
	}
}

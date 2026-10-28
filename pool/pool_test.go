package pool

import (
	"sync"
	"testing"
)

func TestBoundedReuse(t *testing.T) {
	p, err := New(1, func() *int { return new(int) })
	if err != nil {
		t.Fatal(err)
	}
	a, b := p.Get(), p.Get()
	*b = 7
	p.Put(a)
	p.Put(b)
	if *b != 7 {
		t.Fatal("discard mutated caller value")
	}
	if p.Get() != a {
		t.Fatal("cached object not reused")
	}
	p.Put(nil)
	if _, err := New[int](-1, func() *int { return new(int) }); err == nil {
		t.Fatal("negative capacity")
	}
	if _, err := New[int](1, nil); err == nil {
		t.Fatal("nil factory")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				v := p.Get()
				*v = 1
				p.Put(v)
			}
		}()
	}
	wg.Wait()
}

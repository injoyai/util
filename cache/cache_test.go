package cache

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

func TestCycleSnapshots(t *testing.T) {
	c, err := NewCycle[int](3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		c.Append(i)
	}
	if got := c.List(0); !slices.Equal(got, []int{3, 4, 5}) {
		t.Fatal(got)
	}
	snapshot := c.List(2)
	snapshot[0] = 100
	if !slices.Equal(c.List(0), []int{3, 4, 5}) {
		t.Fatal("snapshot modifies ring")
	}
	c.Padding(7)
	c.Append(8)
	if !slices.Equal(c.List(0), []int{7, 7, 8}) {
		t.Fatal(c.List(0))
	}
	c.Clear()
	if c.Len() != 0 || len(c.List(0)) != 0 {
		t.Fatal("clear")
	}
	if _, err := NewCycle[int](0); err == nil {
		t.Fatal("zero capacity")
	}
}

func TestCycleConcurrent(t *testing.T) {
	c, _ := NewCycle[int](10)
	var wg sync.WaitGroup
	for worker := 0; worker < 10; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				c.Append(i)
				_ = c.List(3)
				_ = c.Len()
			}
		}()
	}
	wg.Wait()
	if c.Len() != 10 {
		t.Fatal(c.Len())
	}
}

func TestFileRoundTripAndIsolation(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "nested", "cache.json")
	f, err := NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]int{"n": 1}
	if err := f.Set("value", input); err != nil {
		t.Fatal(err)
	}
	input["n"] = 2
	var first map[string]int
	if ok, err := f.Get("value", &first); !ok || err != nil || first["n"] != 1 {
		t.Fatal(first, err)
	}
	first["n"] = 3
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	f2, err := NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var second map[string]int
	if ok, err := f2.Get("value", &second); !ok || err != nil || second["n"] != 1 {
		t.Fatal(second, err)
	}
	if err := f.Set("bad", make(chan int)); err == nil {
		t.Fatal("marshal error lost")
	}
	if ok, err := f.Get("missing", nil); ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, err := f.Get("value", new(int)); err == nil {
		t.Fatal("decode error lost")
	}
	f.Del("value")
	f.Clear()
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	for _, contents := range []string{"null", "[]", "{"} {
		if err := os.WriteFile(filename, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewFile(filename); err == nil {
			t.Fatal("invalid JSON accepted", contents)
		}
	}
}

func TestFileConcurrent(t *testing.T) {
	f, _ := NewFile(filepath.Join(t.TempDir(), "cache.json"))
	var wg sync.WaitGroup
	for worker := 0; worker < 6; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				if err := f.Set("n", i); err != nil {
					t.Error(err)
				}
				var n int
				_, err := f.Get("n", &n)
				if err != nil {
					t.Error(err)
				}
				if err := f.Save(); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if _, err := NewFile(f.filename); err != nil {
		t.Fatal(err)
	}
}

package str

import (
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestUnicodeAndSplit(t *testing.T) {
	if got := Reverse("Go你好😀"); got != "😀好你oG" {
		t.Fatal(got)
	}
	for _, tc := range []struct {
		s    string
		n    int
		want string
	}{{"你好abc", 2, "你好"}, {"你好", 5, "你好"}, {"", 1, ""}, {"abc", -1, ""}, {"你好abc", 3, "你好a"}} {
		if got := Truncate(tc.s, tc.n); got != tc.want {
			t.Fatalf("Truncate(%q,%d)=%q", tc.s, tc.n, got)
		}
	}
	for _, tc := range []struct {
		index int
		want  string
		ok    bool
	}{{0, "a", true}, {1, "b", true}, {2, "c", true}, {-1, "c", true}, {-3, "a", true}, {3, "", false}, {-4, "", false}} {
		got, ok := SplitAt("a,b,c", ",", tc.index)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("index %d: %q %v", tc.index, got, ok)
		}
	}
	if !HasRepeat("你好你") || HasRepeat("你好") {
		t.Fatal("repeat detection")
	}
	if CropFirst("a:b:c", ":", false) != "b:c" || CropLast("a:b:c", ":", true) != "a:b:" {
		t.Fatal("crop")
	}
}

func TestRandomConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := Random(30, "你我😀")
			if err != nil || utf8.RuneCountInString(s) != 30 || strings.Trim(s, "你我😀") != "" {
				t.Errorf("random %q: %v", s, err)
			}
		}()
	}
	wg.Wait()
	for _, alphabet := range []string{"aa", "\xff"} {
		if _, err := Random(1, alphabet); err == nil {
			t.Fatal("invalid alphabet accepted")
		}
	}
	if _, err := Random(-1, ""); err == nil {
		t.Fatal("negative length accepted")
	}
	if s, err := Random(0, ""); s != "" || err != nil {
		t.Fatal(s, err)
	}
}

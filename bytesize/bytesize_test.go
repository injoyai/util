package bytesize

import (
	"math"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want uint64
	}{{"0", 0}, {"1.5 MiB", 1572864}, {"1GB512MB", GiB + 512*MiB}, {"2 kib", 2 * KiB}, {"18446744073709551615B", math.MaxUint64}, {"0.5B0.5B", 1}} {
		got, err := Parse(tc.s)
		if err != nil || got != tc.want {
			t.Fatalf("%q=%d %v", tc.s, got, err)
		}
	}
	for _, s := range []string{"", "-1MB", "1XY", "NaN", "1.2.3MB", "1e3", "0.5B", "18446744073709551616B", "16EB"} {
		if _, err := Parse(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	if got, err := Speed(MiB, time.Millisecond*500, 1); err != nil || got != "2.0MiB/s" {
		t.Fatal(got, err)
	}
	if _, err := Speed(1, 0, 1); err == nil {
		t.Fatal("zero elapsed")
	}
}

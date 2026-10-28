package csv

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type brokenWriter struct{ err error }

func (w brokenWriter) Write([]byte) (int, error) { return 0, w.err }

func TestReadAndWrite(t *testing.T) {
	for _, s := range []string{"a", "a\n", BOM + "a"} {
		rows, err := Read(strings.NewReader(s))
		if err != nil || len(rows) != 1 || rows[0][0] != "a" {
			t.Fatal(rows, err)
		}
	}
	rows := [][]string{{"中文", "a,b", "line\nnext", "\"quoted\""}, {"short"}}
	var out bytes.Buffer
	if err := Write(&out, rows, true); err != nil {
		t.Fatal(err)
	}
	got, err := Read(&out)
	if err != nil || !reflect.DeepEqual(got, rows) {
		t.Fatal(got, err)
	}
	if _, err := Read(strings.NewReader("\xff\xfe")); err == nil {
		t.Fatal("UTF-16 accepted")
	}
	if _, err := Read(strings.NewReader("\"unclosed")); err == nil {
		t.Fatal("invalid CSV accepted")
	}
	want := errors.New("flush failed")
	if err := Write(brokenWriter{want}, rows, false); !errors.Is(err, want) {
		t.Fatal(err)
	}
	count := 0
	if err := Range(strings.NewReader("a\nb\n"), func(int, []string) bool { count++; return false }); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

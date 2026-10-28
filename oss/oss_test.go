package oss

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	for range 2 {
		if err := New(dir); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("directory: %v, %v", info, err)
	}
	filename := filepath.Join(dir, "more", "file")
	if err := New(filename, "hello", []byte("世界"), nil, strings.NewReader("!")); err != nil {
		t.Fatal(err)
	}
	assertContent(t, filename, "hello世界!")
	info, err = os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0644 {
		t.Fatalf("permissions: %o", info.Mode().Perm())
	}
	if err := New(filename, "short"); err != nil {
		t.Fatal(err)
	}
	assertContent(t, filename, "short")
	if err := New(filename); err == nil {
		t.Fatal("creating a directory over a file must fail")
	}
	assertContent(t, filename, "short")
	if err := New(dir, "content"); err == nil {
		t.Fatal("writing over a directory must fail")
	}
	for i, content := range []any{nil, "", []byte(nil), strings.NewReader("")} {
		filename := filepath.Join(dir, "empty", fmt.Sprint(i))
		if err := New(filename, content); err != nil {
			t.Fatal(err)
		}
		assertContent(t, filename, "")
	}
	if err := New(filename, nil); err != nil {
		t.Fatal(err)
	}
	assertContent(t, filename, "")
}

func TestNewRejectsUnsupportedContentBeforeIO(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "nested", "file")
	input := strings.NewReader("unread")
	err := New(filename, input, 123)
	if err == nil || !strings.Contains(err.Error(), "content[1]") {
		t.Fatalf("unsupported content: %v", err)
	}
	if input.Len() != len("unread") {
		t.Fatal("input consumed before validation")
	}
	if _, err := os.Stat(filepath.Dir(filename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected parent directory: %v", err)
	}
	if err := New(filename, "old"); err != nil {
		t.Fatal(err)
	}
	if err := New(filename, "partial", map[string]int{"n": 1}); err == nil {
		t.Fatal("map must be encoded explicitly")
	}
	assertContent(t, filename, "old")
}

func TestNewReaderFailure(t *testing.T) {
	want := errors.New("read failed")
	for _, exists := range []bool{false, true} {
		t.Run(fmt.Sprint(exists), func(t *testing.T) {
			dir := t.TempDir()
			filename := filepath.Join(dir, "file")
			if exists {
				if err := New(filename, "old"); err != nil {
					t.Fatal(err)
				}
			}
			input := &failingReader{err: want}
			if err := New(filename, "prefix", input, "suffix"); !errors.Is(err, want) {
				t.Fatalf("read error: %v", err)
			}
			if input.closed {
				t.Fatal("caller owns the reader")
			}
			if exists {
				assertContent(t, filename, "old")
			} else if _, err := os.Stat(filename); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("partial destination: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() != "file" {
					t.Fatalf("temporary file leaked: %s", entry.Name())
				}
			}
		})
	}
}

type failingReader struct {
	err    error
	closed bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	return copy(p, "partial"), r.err
}

func (r *failingReader) Close() error {
	r.closed = true
	return nil
}

func assertContent(t *testing.T, filename, want string) {
	t.Helper()
	got, err := os.ReadFile(filename)
	if err != nil || string(got) != want {
		t.Fatalf("content = %q, error = %v; want %q", got, err, want)
	}
}

func TestFailedWritePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "value")
	if err := WriteFile(filename, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	want := errors.New("incomplete input")
	err := Write(filename, 0600, func(w io.Writer) error {
		_, err := w.Write([]byte("partial"))
		if err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filename)
	if string(got) != "old" {
		t.Fatal(string(got))
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("temporary file leaked", entries)
	}
	if err := WriteJSON(filename, make(chan int), 0600); err == nil {
		t.Fatal("marshal error")
	}
	if err := Copy(filename, filename); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(filename)
	if string(got) != "old" {
		t.Fatal(string(got))
	}
	if err := Copy(filename, filepath.Join(dir, "nested", "copy")); err != nil {
		t.Fatal(err)
	}
}

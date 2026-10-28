package httputil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"n":2}`)
		case "/echo":
			_, _ = io.Copy(w, r.Body)
		case "/bad":
			_, _ = io.WriteString(w, `{`)
		case "/status":
			http.Error(w, "private error", http.StatusNotFound)
		case "/empty":
			w.WriteHeader(http.StatusNoContent)
		case "/large":
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, strings.Repeat("x", 100))
		case "/truncated":
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, "partial")
		default:
			_, _ = io.WriteString(w, "download content")
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestJSON(t *testing.T) {
	s := testServer(t)
	c, err := New(s.Client(), 20)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var dst struct {
		N int `json:"n"`
	}
	if err := c.JSON(ctx, http.MethodGet, s.URL+"/json", nil, &dst, nil); err != nil || dst.N != 2 {
		t.Fatal(dst, err)
	}
	h := http.Header{"X-Custom": []string{"original"}}
	if err := c.JSON(ctx, http.MethodPost, s.URL+"/echo", map[string]int{"n": 3}, &dst, h); err != nil || dst.N != 3 {
		t.Fatal(dst, err)
	}
	if h.Get("Content-Type") != "" {
		t.Fatal("headers mutated")
	}
	if err := c.JSON(ctx, http.MethodGet, s.URL+"/empty", nil, &dst, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/bad", "/large", "/truncated"} {
		if err := c.JSON(ctx, http.MethodGet, s.URL+path, nil, &dst, nil); err == nil {
			t.Fatal("error lost", path)
		}
	}
	err = c.JSON(ctx, http.MethodGet, s.URL+"/status", nil, &dst, nil)
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != 404 || strings.Contains(err.Error(), "private") {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := c.JSON(cancelled, http.MethodGet, s.URL+"/json", nil, &dst, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDownloadPreservesExistingFile(t *testing.T) {
	s := testServer(t)
	c, _ := New(s.Client(), 0)
	dir := t.TempDir()
	filename := filepath.Join(dir, "value")
	if err := os.WriteFile(filename, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/large", "/truncated", "/status"} {
		if _, err := c.Download(context.Background(), s.URL+path, filename, 20); err == nil {
			t.Fatal("error lost", path)
		}
		got, _ := os.ReadFile(filename)
		if string(got) != "old" {
			t.Fatal(string(got))
		}
		files, _ := os.ReadDir(dir)
		if len(files) != 1 {
			t.Fatal("temporary files leaked", files)
		}
	}
	// 足够大的上限，确保截断正文错误来自读流，而不是 Content-Length 预检查。
	if _, err := c.Download(context.Background(), s.URL+"/truncated", filename, 200); err == nil {
		t.Fatal("truncated body accepted")
	}
	n, err := c.Download(context.Background(), s.URL+"/file", filename, 100)
	if err != nil || n != 16 {
		t.Fatal(n, err)
	}
	got, _ := os.ReadFile(filename)
	if string(got) != "download content" {
		t.Fatal(string(got))
	}
}

package oss

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUserDataDir(t *testing.T) {
	home := t.TempDir()
	local := filepath.Join(t.TempDir(), "local-data")
	t.Setenv(homeEnv(), home)
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("XDG_DATA_HOME", "")
	var want string
	switch runtime.GOOS {
	case "darwin", "ios":
		want = filepath.Join(home, "Library", "Application Support")
	case "windows":
		want = local
	default:
		want = filepath.Join(home, ".local", "share")
	}
	for _, parts := range [][]string{nil, {"myapp", "data"}} {
		got, err := UserDataDir(parts...)
		expected := filepath.Join(append([]string{want}, parts...)...)
		if err != nil || got != expected {
			t.Fatalf("UserDataDir(%q) = %q, %v; want %q", parts, got, err, expected)
		}
		if _, err := os.Stat(got); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("directory should not be created: %v", err)
		}
	}
	custom := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_DATA_HOME", custom)
	if runtime.GOOS != "darwin" && runtime.GOOS != "ios" && runtime.GOOS != "windows" {
		want = custom
	}
	if got, err := UserDataDir("myapp"); err != nil || got != filepath.Join(want, "myapp") {
		t.Fatalf("XDG override = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(want, "myapp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("application directory should not be created: %v", err)
	}
}

func TestUserDataDirMissingEnvironment(t *testing.T) {
	t.Setenv(homeEnv(), "")
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("XDG_DATA_HOME", "")
	if dir, err := UserDataDir("myapp"); err == nil || dir != "" {
		t.Fatalf("missing environment = %q, %v", dir, err)
	}
}

func TestUserDataDirRelativeBase(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "ios":
		t.Setenv("HOME", "relative/home")
	case "windows":
		t.Setenv("LOCALAPPDATA", "relative/data")
	default:
		t.Setenv("XDG_DATA_HOME", "relative/data")
		t.Setenv(homeEnv(), "relative/home")
	}
	if dir, err := UserDataDir("myapp"); err == nil || dir != "" {
		t.Fatalf("relative base = %q, %v", dir, err)
	}
}

func TestXDGDataDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv(homeEnv(), home)
	custom := filepath.Join(t.TempDir(), "custom")
	for _, input := range []string{"", "relative/data", custom} {
		t.Run(input, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", input)
			want := filepath.Join(home, ".local", "share")
			if input == custom {
				want = custom
			}
			got, err := xdgDataDir()
			if err != nil || got != want {
				t.Fatalf("XDG_DATA_HOME=%q: %q, %v; want %q", input, got, err, want)
			}
		})
	}
	t.Setenv(homeEnv(), "")
	t.Setenv("XDG_DATA_HOME", custom)
	if got, err := xdgDataDir(); err != nil || got != custom {
		t.Fatalf("XDG without home: %q, %v", got, err)
	}
	t.Setenv("XDG_DATA_HOME", "")
	if got, err := xdgDataDir(); err == nil || got != "" {
		t.Fatalf("missing home: %q, %v", got, err)
	}
	t.Setenv(homeEnv(), "relative/home")
	if got, err := xdgDataDir(); err == nil || got != "" {
		t.Fatalf("relative home: %q, %v", got, err)
	}
}

func homeEnv() string {
	switch runtime.GOOS {
	case "windows":
		return "USERPROFILE"
	case "plan9":
		return "home"
	default:
		return "HOME"
	}
}

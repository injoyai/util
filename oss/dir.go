package oss

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// UserDataDir 返回当前用户的数据目录，可追加路径片段，不创建目录。
// macOS 使用 ~/Library/Application Support，Windows 使用 %LOCALAPPDATA%，
// 其他平台使用绝对路径 $XDG_DATA_HOME，未设置、为空或为相对路径时使用 ~/.local/share。
// 无法确定绝对的基础目录时返回错误。parts 按 filepath.Join 拼接，不限制路径范围。
func UserDataDir(parts ...string) (string, error) {
	var dir string
	var err error
	switch runtime.GOOS {
	case "darwin", "ios":
		dir, err = os.UserConfigDir()
	case "windows":
		dir, err = os.UserCacheDir()
	default:
		dir, err = xdgDataDir()
	}
	if err != nil {
		return "", fmt.Errorf("oss: user data directory: %w", err)
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("oss: user data directory is not absolute: %q", dir)
	}
	return filepath.Join(append([]string{dir}, parts...)...), nil
}

func xdgDataDir() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("user home directory is not absolute: %q", home)
	}
	return filepath.Join(home, ".local", "share"), nil
}

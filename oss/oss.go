// Package oss 提供本地文件与目录操作工具。
package oss

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// New 不传 contents 时创建目录（0755）；传入内容时创建或覆盖文件（0644），
// 并自动创建父目录。多个内容按顺序拼接，支持 string、[]byte、io.Reader、nil。
// New(filename, nil) 创建空文件；其他类型返回错误，结构化数据请用 WriteJSON。
// 文件写入沿用 Write 的替换语义：成功前保留原文件，替换后权限为 0644。
// io.Reader 由调用方关闭，发生错误时已读取的数据不会回退。
func New(filename string, contents ...any) error {
	if len(contents) == 0 {
		return os.MkdirAll(filename, 0755)
	}
	// 在创建目录或读取流之前验证全部内容，避免类型错误产生部分副作用。
	for i, content := range contents {
		switch content.(type) {
		case nil, string, []byte, io.Reader:
		default:
			return fmt.Errorf("oss: content[%d]: unsupported type %T", i, content)
		}
	}
	return Write(filename, 0644, func(w io.Writer) error {
		for _, content := range contents {
			var err error
			switch v := content.(type) {
			case string:
				_, err = io.WriteString(w, v)
			case []byte:
				_, err = w.Write(v)
			case io.Reader:
				_, err = io.Copy(w, v)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Write 原子替换文件。临时文件与目标同目录，成功前不改动已有文件。
// perm 必须为权限位。会创建父目录（0755），默认不保留目标原权限。
// Unix 同目录 rename 提供原子可见性；其他平台取决于 os.Rename。
// 文件已 Sync，但不对父目录 Sync，不保证掉电后目录项持久化。
func Write(filename string, perm os.FileMode, write func(io.Writer) error) (err error) {
	if write == nil {
		return errors.New("oss: nil writer callback")
	}
	if err = os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(filename), ".util-*")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if err = f.Chmod(perm.Perm()); err != nil {
		return err
	}
	if err = write(f); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filename)
}

func WriteFile(filename string, data []byte, perm os.FileMode) error {
	return Write(filename, perm, func(w io.Writer) error { _, err := w.Write(data); return err })
}

func WriteJSON(filename string, value any, perm os.FileMode) error {
	return Write(filename, perm, func(w io.Writer) error { e := json.NewEncoder(w); e.SetIndent("", "  "); return e.Encode(value) })
}

// Copy 流式复制普通文件，保留源权限，通过临时文件避免半成品覆盖目标。
func Copy(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("oss: source is not a regular file")
	}
	return Write(dst, info.Mode().Perm(), func(w io.Writer) error { _, err := io.Copy(w, f); return err })
}

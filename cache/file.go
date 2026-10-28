package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/injoyai/util/oss"
)

// File 在编码后的 JSON 层存储值，避免暴露可变 map、指针或切片。
// 同一实例可并发读写；不同实例或进程之间没有写入协调，最后保存者覆盖。
// 必须通过 NewFile 创建，使用后不可复制。Set 不会自动落盘。
type File struct {
	mu       sync.RWMutex
	filename string
	data     map[string]json.RawMessage
}

// NewFile 打开 JSON 对象；文件不存在时返回空存储，损坏文件返回错误。
func NewFile(filename string) (*File, error) {
	if filename == "" {
		return nil, errors.New("cache: empty filename")
	}
	f := &File{filename: filename, data: make(map[string]json.RawMessage)}
	b, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &f.data); err != nil {
		return nil, fmt.Errorf("cache: decode %s: %w", filename, err)
	}
	if f.data == nil {
		return nil, errors.New("cache: expected JSON object, got null")
	}
	return f, nil
}

func (f *File) Set(key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = b
	return nil
}

// Get 将已存在的值解码到 dst；不存在时返回 false, nil。
func (f *File) Get(key string, dst any) (bool, error) {
	f.mu.RLock()
	b, ok := f.data[key]
	f.mu.RUnlock()
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(b, dst)
}

func (f *File) Del(key string) { f.mu.Lock(); defer f.mu.Unlock(); delete(f.data, key) }
func (f *File) Clear()         { f.mu.Lock(); defer f.mu.Unlock(); clear(f.data) }

// Save 持锁保存，保证同一实例的并发 Save 不会用旧快照覆盖新数据。
func (f *File) Save() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return oss.WriteJSON(f.filename, f.data, 0600)
}

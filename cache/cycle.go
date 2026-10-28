// Package cache 提供并发安全的环形缓存和显式保存的 JSON 文件存储。
package cache

import (
	"errors"
	"sync"
)

// Cycle 固定容量，覆盖最旧元素。元素含指针时调用方负责其同步。
// 必须通过 NewCycle 创建，使用后不可复制。
type Cycle[T any] struct {
	mu          sync.RWMutex
	data        []T
	next, count int
}

func NewCycle[T any](capacity int) (*Cycle[T], error) {
	if capacity <= 0 {
		return nil, errors.New("cache: capacity must be positive")
	}
	return &Cycle[T]{data: make([]T, capacity)}, nil
}

func (c *Cycle[T]) Append(value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[c.next] = value
	c.next = (c.next + 1) % len(c.data)
	if c.count < len(c.data) {
		c.count++
	}
}

func (c *Cycle[T]) Len() int { c.mu.RLock(); defer c.mu.RUnlock(); return c.count }

// List 返回从旧到新的独立切片；limit<=0 返回全部，正数保留最近 limit 个。
func (c *Cycle[T]) List(limit int) []T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	n := c.count
	if limit > 0 && limit < n {
		n = limit
	}
	result := make([]T, n)
	start := (c.next - n + len(c.data)) % len(c.data)
	for i := range result {
		result[i] = c.data[(start+i)%len(c.data)]
	}
	return result
}

func (c *Cycle[T]) Clear() { c.mu.Lock(); defer c.mu.Unlock(); clear(c.data); c.next, c.count = 0, 0 }

func (c *Cycle[T]) Padding(value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.data {
		c.data[i] = value
	}
	c.next, c.count = 0, len(c.data)
}

// Package safe 提供并发安全的关闭状态、错误原因和关闭回调管理。
package safe

import (
	"errors"
	"sync"
)

// NewCloserErr 创建已关闭的实例，即使 err 为 nil 也处于关闭状态。
func NewCloserErr(err error) *Closer {
	c := &Closer{closed: true, err: err, done: make(chan struct{})}
	close(c.done)
	return c
}

func NewCloser() *Closer { return &Closer{done: make(chan struct{})} }

// Closer 每个生命周期只接受第一次关闭，并在锁外调用关闭回调。
// 必须通过构造函数创建，不可复制。Done 表示已发起关闭，不表示回调执行完毕。
type Closer struct {
	mu        sync.RWMutex
	closed    bool
	err       error
	closeFunc func(error) error
	done      chan struct{}
}

// Reset 清除关闭状态和原因并创建新的 Done 通道，保留关闭回调。
// 若当前尚未关闭，仅关闭旧 Done 通道，不执行回调。
// 调用方须协调旧资源和回调的退出；Reset 不等待它们，也不会让旧 Done 重新打开。
func (c *Closer) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		close(c.done)
	}
	c.err = nil
	c.done = make(chan struct{})
	c.closed = false
}

func (c *Closer) Done() <-chan struct{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.done
}

// Err 返回本轮首次关闭的原因，不包含关闭回调返回的错误。
func (c *Closer) Err() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.err
}

func (c *Closer) Closed() bool {
	if c == nil {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

func (c *Closer) Close() error { return c.CloseWithErr(errors.New("主动关闭")) }

// CloseWithErr 记录原因、关闭 Done，然后执行回调；返回回调的错误，不恢复 panic。
// 沿用 base 的约定：err 为 nil 或本轮已关闭时不做操作，返回 nil。
// 并发重复关闭不等待首次关闭的回调，回调可重入查询状态或重复 Close。
func (c *Closer) CloseWithErr(err error) error {
	if err == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed, c.err = true, err
	close(c.done)
	fn := c.closeFunc
	c.mu.Unlock()
	if fn != nil {
		return fn(err)
	}
	return nil
}

// SetCloseFunc 替换回调；nil 移除回调。关闭后设置不会补调，Reset 后继续使用。
func (c *Closer) SetCloseFunc(fn func(error) error) *Closer {
	c.mu.Lock()
	c.closeFunc = fn
	c.mu.Unlock()
	return c
}

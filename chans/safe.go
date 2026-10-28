// Package chans 提供可安全关闭的通道、订阅和内存消息流。
package chans

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// NewSafe 创建通道。capacity 可以为零；接收端只通过 Chan 读取。
func NewSafe[T any](capacity int) (*Safe[T], error) {
	if capacity < 0 {
		return nil, errors.New("chans: negative capacity")
	}
	c := make(chan T, capacity)
	return &Safe[T]{Chan: c, c: c, done: make(chan struct{})}, nil
}

// Safe 不可复制。关闭解除阻塞的发送，并允许读取已经缓冲的数据。
// Chan 只读，避免调用方绕过同步直接发送或关闭。
type Safe[T any] struct {
	Chan    <-chan T
	c       chan T
	done    chan struct{}
	once    sync.Once
	mu      sync.RWMutex
	onClose func()
}

func (s *Safe[T]) Done() <-chan struct{} { return s.done }
func (s *Safe[T]) Closed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// Send 等待接收、关闭或上下文取消。发送与关闭同时发生时，允许发送完成。
func (s *Safe[T]) Send(ctx context.Context, value T) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Closed() {
		return io.ErrClosedPipe
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-s.done:
		return io.ErrClosedPipe
	case <-ctx.Done():
		return ctx.Err()
	case s.c <- value:
		return nil
	}
}

// Add 默认阻塞；timeout 为零时尝试发送，负值阻塞，正值限制等待时间。
func (s *Safe[T]) Add(value T, timeouts ...time.Duration) bool {
	timeout := time.Duration(-1)
	if len(timeouts) > 0 {
		timeout = timeouts[0]
	}
	if timeout == 0 {
		return s.Try(value)
	}
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return s.Send(ctx, value) == nil
}

func (s *Safe[T]) Try(value T) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Closed() {
		return false
	}
	select {
	case s.c <- value:
		return true
	default:
		return false
	}
}

func (s *Safe[T]) Timeout(value T, timeout time.Duration) bool { return s.Add(value, timeout) }

func (s *Safe[T]) Close() error {
	s.once.Do(func() {
		close(s.done) // 先释放持读锁的发送者，再关闭数据通道。
		s.mu.Lock()
		close(s.c)
		s.mu.Unlock()
	})
	s.mu.Lock()
	fn := s.onClose
	s.onClose = nil
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
	return nil
}

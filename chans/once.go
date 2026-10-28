package chans

import (
	"context"
	"sync"
)

// WaitOnce 只接受第一次 Done 的值；所有等待者取得同一个值。必须通过 NewWaitOnce 创建。
type WaitOnce[T any] struct {
	once  sync.Once
	done  chan struct{}
	value T
}

func NewWaitOnce[T any]() *WaitOnce[T] { return &WaitOnce[T]{done: make(chan struct{})} }
func (w *WaitOnce[T]) Done(value T)    { w.once.Do(func() { w.value = value; close(w.done) }) }
func (w *WaitOnce[T]) Wait(ctx context.Context) (T, error) {
	select {
	case <-w.done:
		return w.value, nil
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

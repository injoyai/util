package chans

import (
	"context"
	"errors"
	"io"
	"sync"
)

// QueueCover 在满时丢弃最早的数据，保留最近 capacity 条。必须通过 NewQueueCover 创建。
// Append、Close 和多个接收者可并发使用；Close 后可排空已有数据。
type QueueCover[T any] struct {
	Chan   <-chan T
	c      chan T
	mu     sync.Mutex
	closed bool
}

func NewQueueCover[T any](capacity int) (*QueueCover[T], error) {
	if capacity <= 0 {
		return nil, errors.New("chans: queue capacity must be positive")
	}
	c := make(chan T, capacity)
	return &QueueCover[T]{Chan: c, c: c}, nil
}

func (q *QueueCover[T]) Append(value T) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	select {
	case q.c <- value:
	default:
		select {
		case <-q.c:
		default:
		}
		q.c <- value // 生产者已串行化，且通道至少有一个位置。
	}
	return true
}

func (q *QueueCover[T]) Pop(ctx context.Context) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	select {
	case v, ok := <-q.c:
		if !ok {
			return zero, io.EOF
		}
		return v, nil
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}
func (q *QueueCover[T]) Len() int { return len(q.c) }
func (q *QueueCover[T]) Cap() int { return cap(q.c) }
func (q *QueueCover[T]) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.c)
	}
	return nil
}

package chans

import (
	"sync"
	"time"
)

func NewSubscribe[K comparable, V any]() *Subscribe[K, V] {
	return &Subscribe[K, V]{m: make(map[K][]*Safe[V])}
}

// Subscribe 按 topic 广播。消息按发布者调用逐个投递，多个发布者之间不保证顺序。
// 必须使用 NewSubscribe 创建；关闭后新建的订阅立即关闭。
type Subscribe[K comparable, V any] struct {
	mu     sync.RWMutex
	m      map[K][]*Safe[V]
	last   V
	closed bool
}

func (s *Subscribe[K, V]) Last() V { s.mu.RLock(); defer s.mu.RUnlock(); return s.last }

// Publish 的 timeout 为每个订阅者独立的等待预算；0 丢弃满通道的消息，负值阻塞。
// 阻塞发布可通过关闭对应订阅或整个发布器解除。
func (s *Subscribe[K, V]) Publish(topic K, value V, timeout time.Duration) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.last = value
	subscribers := append([]*Safe[V](nil), s.m[topic]...)
	s.mu.Unlock()
	for _, sub := range subscribers {
		sub.Add(value, timeout)
	}
}

func (s *Subscribe[K, V]) Subscribe(topic K, capacity int) (*Safe[V], error) {
	sub, err := NewSafe[V](capacity)
	if err != nil {
		return nil, err
	}
	sub.onClose = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		subscribers := s.m[topic]
		for i, v := range subscribers {
			if v == sub {
				copy(subscribers[i:], subscribers[i+1:])
				subscribers[len(subscribers)-1] = nil
				subscribers = subscribers[:len(subscribers)-1]
				if len(subscribers) == 0 {
					delete(s.m, topic)
				} else {
					s.m[topic] = subscribers
				}
				break
			}
		}
	}
	s.mu.Lock()
	closed := s.closed
	if !closed {
		s.m[topic] = append(s.m[topic], sub)
	}
	s.mu.Unlock()
	if closed {
		sub.Close()
	}
	return sub, nil
}

func (s *Subscribe[K, V]) Close() error {
	s.mu.Lock()
	s.closed = true
	subscribers := s.m
	s.m = make(map[K][]*Safe[V])
	s.mu.Unlock()
	for _, group := range subscribers {
		for _, sub := range group {
			sub.Close()
		}
	}
	return nil
}

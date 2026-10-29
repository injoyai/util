// Package wait 按 key 关联请求与响应，支持排队、共享响应和有限次数回调。
package wait

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Default 创建 key 和响应值均为 any 的独立等待器，默认等待超时为 10 秒。
func Default() *Waiter[any, any] { return New[any, any](10 * time.Second) }

// New 创建按 key 关联请求与响应的等待器。
func New[K comparable, V any](timeout time.Duration) *Waiter[K, V] {
	return &Waiter[K, V]{m: make(map[K]*pending[V]), timeout: timeout}
}

// Waiter 按 key 管理响应等待与回调，必须通过 New 或 Default 创建。
// 同步超时包含排队时间；异步回调在 Done 的调用方执行且可重入。
// 多个 Done 可并发执行回调，回调须自行同步共享状态。
type Waiter[K comparable, V any] struct {
	mu      sync.RWMutex
	m       map[K]*pending[V]
	timeout time.Duration
	reuse   bool
	closed  bool
}

func (this *Waiter[K, V]) SetReuse(b ...bool) *Waiter[K, V] {
	this.mu.Lock()
	this.reuse = len(b) == 0 || b[0]
	this.mu.Unlock()
	return this
}

func (this *Waiter[K, V]) SetTimeout(timeout time.Duration) *Waiter[K, V] {
	this.mu.Lock()
	this.timeout = timeout
	this.mu.Unlock()
	return this
}

func (this *Waiter[K, V]) IsWait(key K) bool {
	this.mu.RLock()
	defer this.mu.RUnlock()
	_, ok := this.m[key]
	return ok
}

func (this *Waiter[K, V]) Wait(key K, timeouts ...time.Duration) (V, error) {
	return this.Sync(key, timeouts...)
}

// Sync waits for one response. With reuse enabled, the next response is shared.
// The timeout includes time spent queued behind earlier waiters.
func (this *Waiter[K, V]) Sync(key K, timeouts ...time.Duration) (V, error) {
	this.mu.Lock()
	if this.closed {
		this.mu.Unlock()
		var zero V
		return zero, ErrClosed
	}
	timeout := this.timeout
	if len(timeouts) > 0 {
		timeout = timeouts[0]
	}
	if timeout <= 0 {
		this.mu.Unlock()
		var zero V
		return zero, context.DeadlineExceeded
	}
	w := &syncWaiter[V]{result: make(chan data[V], 1), reuse: this.reuse}
	p := this.pending(key)
	p.waiters = append(p.waiters, w)
	this.mu.Unlock()
	var deadline <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		deadline = timer.C
	}
	select {
	case result := <-w.result:
		return result.value, result.err
	case <-deadline:
		this.mu.Lock()
		if p := this.m[key]; p != nil {
			for i, v := range p.waiters {
				if v == w {
					copy(p.waiters[i:], p.waiters[i+1:])
					p.waiters[len(p.waiters)-1] = nil
					p.waiters = p.waiters[:len(p.waiters)-1]
					break
				}
			}
			this.removeEmpty(key, p)
		}
		this.mu.Unlock()
		var zero V
		return zero, context.DeadlineExceeded
	}
}

// Async 注册回调。num>0 表示次数，num<0 表示无限次，num=0 不注册。
// timeout<=0 表示不自动过期；Close 会清理注册和定时器。
func (this *Waiter[K, V]) Async(key K, fn Handler[V], num int, timeouts ...time.Duration) {
	if num == 0 || fn == nil {
		return
	}
	this.mu.Lock()
	if this.closed {
		this.mu.Unlock()
		return
	}
	timeout := this.timeout
	if len(timeouts) > 0 {
		timeout = timeouts[0]
	}
	item := &asyncItem[V]{fn: fn, remaining: num}
	if timeout > 0 {
		item.deadline = time.Now().Add(timeout)
		item.timer = time.AfterFunc(timeout, func() {
			this.mu.Lock()
			defer this.mu.Unlock()
			if p := this.m[key]; p != nil {
				for i, v := range p.callbacks {
					if v == item {
						copy(p.callbacks[i:], p.callbacks[i+1:])
						p.callbacks[len(p.callbacks)-1] = nil
						p.callbacks = p.callbacks[:len(p.callbacks)-1]
						break
					}
				}
				this.removeEmpty(key, p)
			}
		})
	}
	p := this.pending(key)
	p.callbacks = append(p.callbacks, item)
	this.mu.Unlock()
}

func (this *Waiter[K, V]) Done(key K, value V, errs ...error) bool {
	var err error
	if len(errs) > 0 {
		err = errs[0]
	}
	this.mu.Lock()
	p, ok := this.m[key]
	if !ok {
		this.mu.Unlock()
		return false
	}
	delivered := false
	if len(p.waiters) > 0 {
		n := 1
		if p.waiters[0].reuse {
			for n < len(p.waiters) && p.waiters[n].reuse {
				n++
			}
		}
		for _, w := range p.waiters[:n] {
			w.result <- data[V]{value: value, err: err}
		}
		copy(p.waiters, p.waiters[n:])
		for i := len(p.waiters) - n; i < len(p.waiters); i++ {
			p.waiters[i] = nil
		}
		p.waiters = p.waiters[:len(p.waiters)-n]
		delivered = true
	}
	callbacks := make([]Handler[V], 0, len(p.callbacks))
	remaining := p.callbacks[:0]
	now := time.Now()
	for _, item := range p.callbacks {
		if !item.deadline.IsZero() && !now.Before(item.deadline) {
			item.timer.Stop()
			continue
		}
		delivered = true
		callbacks = append(callbacks, item.fn)
		if item.remaining > 0 {
			item.remaining--
		}
		if item.remaining != 0 {
			remaining = append(remaining, item)
		} else if item.timer != nil {
			item.timer.Stop()
		}
	}
	for i := len(remaining); i < len(p.callbacks); i++ {
		p.callbacks[i] = nil
	}
	p.callbacks = remaining
	this.removeEmpty(key, p)
	this.mu.Unlock()
	// Callbacks may register new waits or complete another response.
	for _, fn := range callbacks {
		if fn != nil {
			fn(value, err)
		}
	}
	return delivered
}

func (this *Waiter[K, V]) pending(key K) *pending[V] {
	p := this.m[key]
	if p == nil {
		p = &pending[V]{}
		this.m[key] = p
	}
	return p
}

func (this *Waiter[K, V]) removeEmpty(key K, p *pending[V]) {
	if len(p.waiters) == 0 && len(p.callbacks) == 0 {
		delete(this.m, key)
	}
}

type Handler[V any] func(V, error)
type data[V any] struct {
	value V
	err   error
}
type pending[V any] struct {
	waiters   []*syncWaiter[V]
	callbacks []*asyncItem[V]
}
type syncWaiter[V any] struct {
	result chan data[V]
	reuse  bool
}
type asyncItem[V any] struct {
	fn        Handler[V]
	remaining int
	deadline  time.Time
	timer     *time.Timer
}

// ErrClosed 表示等待器已关闭。
var ErrClosed = errors.New("wait: closed")

// Close 唤醒同步等待者并停止回调定时器，不执行尚未触发的异步回调。
// 已被 Done 取出的回调可以继续执行；需要等待它们时由调用方同步。
func (this *Waiter[K, V]) Close() error {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.closed = true
	for _, p := range this.m {
		for _, w := range p.waiters {
			w.result <- data[V]{err: ErrClosed}
		}
		for _, item := range p.callbacks {
			if item.timer != nil {
				item.timer.Stop()
			}
		}
	}
	clear(this.m)
	return nil
}

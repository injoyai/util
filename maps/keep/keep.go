// Package keep 检查最近最多六次心跳是否连续。
package keep

import (
	"sync"
	"time"
)

func New[K comparable](timeout time.Duration) *Keep[K] {
	return &Keep[K]{m: map[K]*info{}, timeout: timeout}
}

// Keep 保留每个 key 的六次心跳；不再监测的 key 应使用 Del 移除。
// 回调在锁外执行，可以重入；并发 Keep 时，回调本身也可能并发执行。
type Keep[K comparable] struct {
	m       map[K]*info
	mu      sync.RWMutex
	timeout time.Duration
	onKeep  func(K, time.Time)
}

func (this *Keep[K]) SetTimeout(timeout time.Duration) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.timeout = timeout
}

func (this *Keep[K]) OnKeep(f func(K, time.Time)) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.onKeep = f
}

func (this *Keep[K]) Keeping(key K, n ...int) bool {
	this.mu.RLock()
	defer this.mu.RUnlock()
	_info, ok := this.m[key]
	if !ok || _info == nil {
		return false
	}
	return _info.keeping(this.timeout, n...)
}

func (this *Keep[T]) Keep(key T) {
	this.mu.Lock()
	_info, ok := this.m[key]
	if !ok {
		_info = &info{}
		this.m[key] = _info
	}
	now := time.Now()
	_info.keep(now)
	fn := this.onKeep
	this.mu.Unlock()
	if fn != nil {
		fn(key, now)
	}
}

const _len = 6

type info struct {
	nodes [_len]time.Time
}

func (this *info) keep(t time.Time) {
	copy(this.nodes[:], this.nodes[1:])
	this.nodes[_len-1] = t
}

func (this *info) keeping(timeout time.Duration, n ...int) bool {
	_n := 1
	if len(n) > 0 {
		_n = n[0]
	}
	if _n <= 0 || _n > _len {
		return false
	}
	for i := 0; i < _n && i < _len; i++ {
		if i == 0 {
			if this.nodes[_len-1].IsZero() || time.Since(this.nodes[_len-1]) > timeout {
				return false
			}
		} else {
			if this.nodes[_len-i-1].IsZero() || this.nodes[_len-i].Sub(this.nodes[_len-i-1]) > timeout {
				return false
			}
		}
	}
	return true
}

// Del 删除不再需要监测的 key。
func (this *Keep[K]) Del(key K) { this.mu.Lock(); delete(this.m, key); this.mu.Unlock() }

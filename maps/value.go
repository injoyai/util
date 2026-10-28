package maps

import (
	"sync"
	"time"
)

type expiryValue[V any] struct {
	Value V // 创建后不再修改；本次选定的返回值不受随后过期影响。
	valid int64
	mu    sync.RWMutex
}

func (this *expiryValue[V]) Val() (V, bool) {
	this.mu.RLock()
	defer this.mu.RUnlock()
	if this.validNow() {
		return this.Value, true
	}
	var zero V
	return zero, false
}

func (this *expiryValue[V]) validNow() bool {
	return this.valid <= 0 || this.valid > time.Now().UnixNano()
}

func (this *expiryValue[V]) Valid() bool {
	this.mu.RLock()
	defer this.mu.RUnlock()
	return this.validNow()
}

func (this *expiryValue[V]) SetExpiration(expiration ...time.Duration) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if !this.validNow() {
		return
	}
	this.setExpiration(expiration...)
}

func (this *expiryValue[V]) setExpiration(expiration ...time.Duration) {
	this.valid = 0
	if len(expiration) > 0 && expiration[0] > 0 {
		this.valid = time.Now().Add(expiration[0]).UnixNano()
	}
}

func (this *expiryValue[V]) clone() *expiryValue[V] {
	this.mu.RLock()
	defer this.mu.RUnlock()
	return &expiryValue[V]{Value: this.Value, valid: this.valid}
}

func newValue[V any](value V, expiration ...time.Duration) *expiryValue[V] {
	v := &expiryValue[V]{Value: value}
	v.setExpiration(expiration...)
	return v
}

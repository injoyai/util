package maps

import "sync"

func newStorage[K comparable, V any]() *storage[K, V] {
	return &storage[K, V]{m: make(map[K]V)}
}

type storage[K comparable, V any] struct {
	m  map[K]V
	mu sync.RWMutex
}

func (this *storage[K, V]) Set(key K, value V) {
	this.mu.Lock()
	this.m[key] = value
	this.mu.Unlock()
}

func (this *storage[K, V]) Get(key K) (V, bool) {
	this.mu.RLock()
	value, ok := this.m[key]
	this.mu.RUnlock()
	return value, ok
}

func (this *storage[K, V]) Del(key K) {
	this.mu.Lock()
	delete(this.m, key)
	this.mu.Unlock()
}

func (this *storage[K, V]) Range(fn func(K, V) bool) {
	this.mu.RLock()
	type entry struct {
		key   K
		value V
	}
	snapshot := make([]entry, 0, len(this.m))
	for k, v := range this.m {
		snapshot = append(snapshot, entry{key: k, value: v})
	}
	this.mu.RUnlock()
	for _, item := range snapshot {
		if !fn(item.key, item.value) {
			break
		}
	}
}

func (this *storage[K, V]) take(key K) (V, bool) {
	this.mu.Lock()
	defer this.mu.Unlock()
	value, ok := this.m[key]
	delete(this.m, key)
	return value, ok
}

func (this *storage[K, V]) swap(key K, value V) (V, bool) {
	this.mu.Lock()
	defer this.mu.Unlock()
	old, ok := this.m[key]
	this.m[key] = value
	return old, ok
}

func (this *storage[K, V]) loadOrStore(key K, value V, valid func(V) bool) (V, bool) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if old, ok := this.m[key]; ok && valid(old) {
		return old, true
	}
	this.m[key] = value
	return value, false
}

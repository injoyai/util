// Package maps 提供泛型并发 Map 和稀疏位图。
package maps

import (
	"context"
	"sync"
	"time"

	"github.com/injoyai/util/chans"
)

func NewGeneric[K comparable, V any]() *Generic[K, V] {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Generic[K, V]{
		clearCtx: ctx, clearCancel: cancel,
		locks: make(map[K]*keyLock),
		m:     newStorage[K, *expiryValue[V]](),
	}
	return e
}

// Generic 是支持过期、原子操作、快照遍历和订阅的并发 Map。
// 必须通过 NewGeneric 创建；复制的值内部的指针由调用方负责同步。
type Generic[K comparable, V any] struct {
	m           *storage[K, *expiryValue[V]] //接口
	keyMu       sync.Mutex
	locks       map[K]*keyLock
	eventMu     sync.RWMutex
	clearCtx    context.Context
	clearCancel context.CancelFunc
	clearMu     sync.Mutex
	clearWG     sync.WaitGroup
	clearOnce   sync.Once //清理过期数据,单次执行

	onGet []func(K, V, bool) //读取数据事件
	onSet []func(K, V)       //设置数据事件
	onDel []func(K)          //删除数据事件

	subscribe     *chans.Subscribe[K, V]
	subscribeOnce sync.Once
}

// Has 是否存在
func (this *Generic[K, V]) Has(key K) bool {
	_, has := this.Get(key)
	return has
}

// Get 获取数据
func (this *Generic[K, V]) Get(key K) (V, bool) {
	val, has := this.m.Get(key)
	if has {
		v, exist := val.Val()
		this._onGet(key, v, exist)
		return v, exist
	}
	var zero V
	this._onGet(key, zero, false)
	return zero, false
}

func (this *Generic[K, V]) _onGet(key K, value V, exist bool) {
	this.eventMu.RLock()
	callbacks := this.onGet
	this.eventMu.RUnlock()
	for _, f := range callbacks {
		if f != nil {
			f(key, value, exist)
		}
	}
}

// Set 设置数据,可选有效期
func (this *Generic[K, V]) Set(key K, value V, expiration ...time.Duration) {
	this.m.Set(key, newValue[V](value, expiration...))
	//设置数据事件
	this.notifySet(key, value)
}

func (this *Generic[K, V]) notifySet(key K, value V) {
	this.eventMu.RLock()
	callbacks := this.onSet
	this.eventMu.RUnlock()
	for _, f := range callbacks {
		if f != nil {
			f(key, value)
		}
	}
}

// SetExpiration 设置有效期
func (this *Generic[K, V]) SetExpiration(key K, expiration time.Duration) bool {
	val, has := this.m.Get(key)
	if has {
		val.SetExpiration(expiration)
	}
	return has
}

// Del 删除键
func (this *Generic[K, V]) Del(key K) {
	this.m.Del(key)
	//删除事件
	this.notifyDel(key)
}

func (this *Generic[K, V]) notifyDel(key K) {
	this.eventMu.RLock()
	callbacks := this.onDel
	this.eventMu.RUnlock()
	for _, f := range callbacks {
		if f != nil {
			f(key)
		}
	}
}

// GetAndDel 获取数据,并删除数据
func (this *Generic[K, V]) GetAndDel(key K) (V, bool) {
	value, has := this.m.take(key)
	var result V
	if has {
		result, has = value.Val()
	}
	this._onGet(key, result, has)
	if has {
		this.notifyDel(key)
	}
	return result, has
}

// GetAndSet 获取老数据,并设置新数据
func (this *Generic[K, V]) GetAndSet(key K, value V, expiration ...time.Duration) (V, bool) {
	old, has := this.m.swap(key, newValue(value, expiration...))
	var result V
	if has {
		result, has = old.Val()
	}
	this._onGet(key, result, has)
	this.notifySet(key, value)
	return result, has
}

// GetOrSet 尝试获取数据,存在则返回数据,不存在的话存储传入的值,并返回出去,一般使用GetOrSetByHandler
func (this *Generic[K, V]) GetOrSet(key K, value V, expiration ...time.Duration) (V, bool) {
	if value, has := this.Get(key); has {
		return value, true
	}
	stored, has := this.m.loadOrStore(key, newValue(value, expiration...), func(v *expiryValue[V]) bool { return v.Valid() })
	result := stored.Value
	if !has {
		this.notifySet(key, value)
	}
	return result, has
}

// GetOrSetByHandler2
// 尝试获取数据,存在则直接返回数据,
// 不存在的话调用函数,生成数据,储存并返回最新数据
// 执行函数时,增加了锁,避免并发,瞬时大量请求
// check-lock-check
func (this *Generic[K, V]) GetOrSetByHandler2(key K, handler func() (V, error), expiration ...time.Duration) (V, error) {
	val, has := this.Get(key)
	if has || handler == nil {
		return val, nil
	}
	var stored bool
	var err error
	func() {
		unlock := this.lockKey(key)
		defer unlock()
		if old, ok := this.m.Get(key); ok {
			if result, valid := old.Val(); valid {
				val = result
				return
			}
		}
		var value V
		value, err = handler()
		if err != nil {
			return
		}
		result, existed := this.m.loadOrStore(key, newValue(value, expiration...), func(v *expiryValue[V]) bool { return v.Valid() })
		val = result.Value
		stored = !existed
	}()
	if stored {
		this.notifySet(key, val)
	}
	return val, err
}

// GetOrSetByHandler 接受无错误返回值的工厂，沿用 GetOrSetByHandler2 的缓存和并发语义。
// handler 为 nil 时仅读取缓存，不存在则返回零值。
func (this *Generic[K, V]) GetOrSetByHandler(key K, handler func() V, expiration ...time.Duration) V {
	if handler == nil {
		value, _ := this.Get(key)
		return value
	}
	value, _ := this.GetOrSetByHandler2(key, func() (V, error) { return handler(), nil }, expiration...)
	return value
}

// Range 遍历数据,返回false结束遍历
func (this *Generic[K, V]) Range(fn func(key K, value V) bool) {
	this.m.Range(func(key K, value *expiryValue[V]) bool {
		v, valid := value.Val()
		return !valid || fn(key, v)
	})
}

// Map 返回有效数据的独立 map[K]V 快照。
func (this *Generic[K, V]) Map() map[K]V {
	m := map[K]V{}
	this.Range(func(key K, value V) bool {
		m[key] = value
		return true
	})
	return m
}

// Clone 复制数据和过期时间；不复制事件回调、订阅或自动清理任务。
func (this *Generic[K, V]) Clone() *Generic[K, V] {
	m := NewGeneric[K, V]()
	this.m.Range(func(key K, value *expiryValue[V]) bool {
		m.m.Set(key, value.clone())
		return true
	})
	return m
}

// Clear 清除过期数据
func (this *Generic[K, V]) Clear() {
	this.m.mu.Lock()
	defer this.m.mu.Unlock()
	for key, value := range this.m.m {
		if !value.Valid() {
			delete(this.m.m, key)
		}
	}
}

// RunClear 定时清理过期数据
func (this *Generic[K, V]) RunClear(interval time.Duration) *Generic[K, V] {
	if interval <= 0 {
		return this
	}
	this.clearMu.Lock()
	defer this.clearMu.Unlock()
	if this.clearCtx.Err() != nil {
		return this
	}
	this.clearOnce.Do(func() {
		this.clearWG.Add(1)
		go func() {
			defer this.clearWG.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-this.clearCtx.Done():
					return
				case <-ticker.C:
					this.Clear()
				}
			}
		}()
	})
	return this
}

func (this *Generic[K, V]) OnGet(f func(k K, v V, exist bool)) {
	this.eventMu.Lock()
	defer this.eventMu.Unlock()
	this.onGet = append(this.onGet, f)
}

func (this *Generic[K, V]) OnSet(f func(k K, v V)) {
	this.eventMu.Lock()
	defer this.eventMu.Unlock()
	this.onSet = append(this.onSet, f)
}

func (this *Generic[K, V]) OnDel(f func(k K)) {
	this.eventMu.Lock()
	defer this.eventMu.Unlock()
	this.onDel = append(this.onDel, f)
}

// Subscribe 订阅指定key的数据
func (this *Generic[K, V]) Subscribe(key K, capacity int) (*chans.Safe[V], error) {
	this.subscribeOnce.Do(func() {
		sub := chans.NewSubscribe[K, V]()
		this.eventMu.Lock()
		this.subscribe = sub
		this.onSet = append(this.onSet, func(k K, v V) { sub.Publish(k, v, 0) })
		this.eventMu.Unlock()
		if this.clearCtx.Err() != nil {
			sub.Close()
		}
	})
	return this.subscribe.Subscribe(key, capacity)
}

// Close stops automatic cleanup and closes subscribers.
func (this *Generic[K, V]) Close() error {
	this.clearMu.Lock()
	this.clearCancel()
	this.clearMu.Unlock()
	this.clearWG.Wait()
	this.eventMu.RLock()
	sub := this.subscribe
	this.eventMu.RUnlock()
	if sub != nil {
		return sub.Close()
	}
	return nil
}

type keyLock struct {
	mu    sync.Mutex
	users int
}

func (this *Generic[K, V]) lockKey(key K) func() {
	this.keyMu.Lock()
	lock := this.locks[key]
	if lock == nil {
		lock = &keyLock{}
		this.locks[key] = lock
	}
	lock.users++
	this.keyMu.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		this.keyMu.Lock()
		lock.users--
		if lock.users == 0 {
			delete(this.locks, key)
		}
		this.keyMu.Unlock()
	}
}

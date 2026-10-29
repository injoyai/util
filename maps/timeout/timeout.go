// Package timeout 在调用方管理的 Run 循环中检测心跳超时。
package timeout

import (
	"context"
	"sync"
	"time"
)

// Default 创建 key 为 any 的独立心跳超时管理器，超时 1 分钟，每 10 秒检查一次。
func Default() *Timeout[any] { return New[any](time.Minute, 10*time.Second) }

// New 创建按 key 检测心跳超时的管理器，依次指定超时时长和检查间隔。
// interval 非正时使用 10 秒；timeout 非正时在下次检查中视为超时。
func New[K comparable](timeout, interval time.Duration) *Timeout[K] {
	x := &Timeout[K]{interval: 10 * time.Second, timeout: timeout, m: make(map[K]time.Time)}
	return x.SetInterval(interval)
}

// Timeout 管理各 key 的心跳及超时回调，必须通过 New 或 Default 创建。
// 超时检查由调用方运行 Run 并通过上下文取消。
type Timeout[K comparable] struct {
	mu          sync.Mutex
	interval    time.Duration
	timeout     time.Duration
	timeoutFunc func(K) error
	m           map[K]time.Time
}

func (this *Timeout[K]) SetDealFunc(fn func(K) error) *Timeout[K] {
	this.mu.Lock()
	this.timeoutFunc = fn
	this.mu.Unlock()
	return this
}

func (this *Timeout[K]) SetTimeout(timeout time.Duration) *Timeout[K] {
	this.mu.Lock()
	this.timeout = timeout
	this.mu.Unlock()
	return this
}

// SetInterval ignores nonpositive intervals to avoid a busy loop.
func (this *Timeout[K]) SetInterval(interval time.Duration) *Timeout[K] {
	this.mu.Lock()
	if interval > 0 {
		this.interval = interval
	}
	this.mu.Unlock()
	return this
}

func (this *Timeout[K]) Keep(key K) {
	this.mu.Lock()
	this.m[key] = time.Now()
	this.mu.Unlock()
}

func (this *Timeout[K]) Del(key K) {
	this.mu.Lock()
	delete(this.m, key)
	this.mu.Unlock()
}

// Run 阻塞运行超时检查，直到 ctx 取消。一个实例只应运行一个 Run。
// 回调在锁外执行，返回错误时保留 key 以便下次重试；回调必须及时返回。
// 每次回调前复核心跳和超时配置；已确认的回调不会被随后的 Keep 撤回。
func (this *Timeout[K]) Run(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	for {
		this.mu.Lock()
		interval := this.interval
		this.mu.Unlock()
		timer.Reset(interval)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		this.mu.Lock()
		now, timeout := time.Now(), this.timeout
		expired := make(map[K]time.Time)
		for key, last := range this.m {
			if now.Sub(last) > timeout {
				expired[key] = last
			}
		}
		this.mu.Unlock()
		for key, last := range expired {
			if ctx.Err() != nil {
				return
			}
			this.mu.Lock()
			current, exists := this.m[key]
			valid := exists && current == last && time.Since(current) > this.timeout
			fn := this.timeoutFunc
			this.mu.Unlock()
			if !valid {
				continue
			}
			if fn != nil && fn(key) != nil {
				continue
			}
			this.mu.Lock()
			// A heartbeat during the callback belongs to a new timeout period.
			if current, ok := this.m[key]; ok && current == last {
				delete(this.m, key)
			}
			this.mu.Unlock()
		}
	}
}

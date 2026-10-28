package task

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

func NewOneRun(fn func(context.Context) error) (*OneRun, error) {
	if fn == nil {
		return nil, errors.New("task: nil handler")
	}
	return &OneRun{fn: fn}, nil
}

// OneRun 的每次 Run 都取消并等待上次执行，再运行本次任务。fn 必须响应 ctx。
// 必须通过 NewOneRun 创建；不会恢复 panic，也不会启动常驻后台协程。
type OneRun struct {
	cancel  context.CancelFunc
	running uint32
	mu      sync.Mutex
	startMu sync.Mutex
	fn      func(ctx context.Context) error
	done    chan struct{}
}

func (this *OneRun) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	this.startMu.Lock()
	this.mu.Lock()
	cancel, done := this.cancel, this.done
	this.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	if err := ctx.Err(); err != nil {
		this.startMu.Unlock()
		return err
	}
	ctx, cancel = context.WithCancel(ctx)
	done = make(chan struct{})
	this.mu.Lock()
	this.cancel, this.done = cancel, done
	atomic.StoreUint32(&this.running, 1)
	this.mu.Unlock()
	this.startMu.Unlock()
	defer func() {
		cancel()
		this.mu.Lock()
		this.cancel = nil
		atomic.StoreUint32(&this.running, 0)
		close(done)
		this.mu.Unlock()
	}()
	return this.fn(ctx)
}

func (this *OneRun) Running() bool {
	return atomic.LoadUint32(&this.running) == 1
}

// Stop 取消并等待当前任务退出；不阻止后续 Run。不得在 fn 内调用。
func (this *OneRun) Stop() {
	this.mu.Lock()
	cancel, done := this.cancel, this.done
	this.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

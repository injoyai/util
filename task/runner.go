package task

import (
	"context"
	"errors"
	"sync"
)

var ErrRunning = errors.New("task: already running")

// Runner 管理一个可重复启动的任务。取消依赖 fn 响应 ctx；不会恢复 panic。
// 必须通过 NewRunner 创建。Done 表示本轮执行已经退出，Stop 会取消并等待退出。
type Runner struct {
	mu      sync.Mutex
	fn      func(context.Context) error
	cancel  context.CancelFunc
	done    chan struct{}
	running bool
	err     error
}

func NewRunner(fn func(context.Context) error) (*Runner, error) {
	if fn == nil {
		return nil, errors.New("task: nil handler")
	}
	done := make(chan struct{})
	close(done)
	return &Runner{fn: fn, done: done}, nil
}
func (r *Runner) prepare(ctx context.Context) (func() error, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil, ErrRunning
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	r.cancel, r.done, r.running, r.err = cancel, done, true, nil
	return func() (err error) {
		defer func() {
			cancel()
			r.mu.Lock()
			r.err = err
			r.running = false
			r.cancel = nil
			close(done)
			r.mu.Unlock()
		}()
		return r.fn(ctx)
	}, nil
}
func (r *Runner) Run(ctx context.Context) error {
	fn, err := r.prepare(ctx)
	if err != nil {
		return err
	}
	return fn()
}

// Start 在返回前安装取消函数，避免紧随其后的 Stop 漏掉任务。错误通过 Err 获取。
func (r *Runner) Start(ctx context.Context) error {
	fn, err := r.prepare(ctx)
	if err != nil {
		return err
	}
	go fn()
	return nil
}

// Stop 取消并等待当前任务退出；不阻止后续 Start。不得在 fn 内调用。
func (r *Runner) Stop() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	<-done
}
func (r *Runner) Done() <-chan struct{} { r.mu.Lock(); defer r.mu.Unlock(); return r.done }
func (r *Runner) Running() bool         { r.mu.Lock(); defer r.mu.Unlock(); return r.running }
func (r *Runner) Err() error            { r.mu.Lock(); defer r.mu.Unlock(); return r.err }

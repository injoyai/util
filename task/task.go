// Package task 提供可取消的重试和有并发上限的批量处理。
package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Retry 首次执行后最多重试 retries 次。delay 在每次重试前调用，
// 参数从 1 开始；nil 或非正等待时间表示立即重试。
// fn 必须响应 ctx；函数不会强行终止正在运行的 fn，也不恢复 panic。
func Retry(ctx context.Context, retries int, delay func(int) time.Duration, fn func(context.Context) error) error {
	if retries < 0 || fn == nil {
		return errors.New("task: invalid retries or nil handler")
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 && delay != nil {
			if d := delay(attempt); d > 0 {
				t := time.NewTimer(d)
				select {
				case <-ctx.Done():
					t.Stop()
					return ctx.Err()
				case <-t.C:
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil || attempt == retries {
			return err
		}
	}
}

type Result[T any] struct {
	Value T
	Err   error
}

// Map 按输入顺序返回结果，所有任务错误通过 errors.Join 汇总。
// limit 必须>0。取消时停止派发，并等待已启动任务退出。
// handler 并发调用，需自行同步其共享状态并响应 ctx。
// 输入切片在执行期间不得修改。不会恢复 handler 的 panic。
func Map[T, R any](ctx context.Context, input []T, limit int, handler func(context.Context, T) (R, error)) ([]Result[R], error) {
	if limit <= 0 || handler == nil {
		return nil, errors.New("task: positive limit and non-nil handler required")
	}
	results := make([]Result[R], len(input))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < min(limit, len(input)); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if err := ctx.Err(); err != nil {
					results[i].Err = err
					continue
				}
				results[i].Value, results[i].Err = handler(ctx, input[i])
			}
		}()
	}
	for i := range input {
		if err := ctx.Err(); err != nil {
			for j := i; j < len(input); j++ {
				results[j].Err = err
			}
			break
		}
		select {
		case jobs <- i:
		case <-ctx.Done():
			for j := i; j < len(input); j++ {
				results[j].Err = ctx.Err()
			}
			close(jobs)
			wg.Wait()
			return results, joinErrors(ctx, results)
		}
	}
	close(jobs)
	wg.Wait()
	return results, joinErrors(ctx, results)
}

func joinErrors[T any](ctx context.Context, results []Result[T]) error {
	errs := []error{ctx.Err()}
	for i, result := range results {
		if result.Err != nil {
			errs = append(errs, fmt.Errorf("task %d: %w", i, result.Err))
		}
	}
	return errors.Join(errs...)
}

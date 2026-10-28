// Package pool 提供不会被 GC 自动清空的有界对象复用池。
package pool

import "errors"

// Pool 只限制闲置对象数量，不限制同时借出的对象数量；对象归还后不可再使用。
// 池不重置对象，调用方负责清理引用和敏感内容。必须通过 New 创建。
type Pool[T any] struct {
	items   chan *T
	factory func() *T
}

func New[T any](capacity int, factory func() *T) (*Pool[T], error) {
	if capacity < 0 || factory == nil {
		return nil, errors.New("pool: invalid capacity or nil factory")
	}
	return &Pool[T]{items: make(chan *T, capacity), factory: factory}, nil
}
func (p *Pool[T]) Get() *T {
	select {
	case v := <-p.items:
		return v
	default:
		return p.factory()
	}
}

// Put 在池满时放弃复用，不修改对象本身。nil 会被忽略。
func (p *Pool[T]) Put(value *T) {
	if value != nil {
		select {
		case p.items <- value:
		default:
		}
	}
}

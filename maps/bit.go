package maps

import (
	"sync"
	"sync/atomic"
)

// Bit 是并发稀疏位图，零值可用。清零后保留分组，避免删除分组时丢失并发置位。
// 适合键空间相对稳定的状态标志；每个曾访问的分组会保留到整个 Bit 被释放。
type Bit struct{ groups sync.Map }

func (b *Bit) Get(key uint64) bool {
	v, ok := b.groups.Load(key / 64)
	return ok && v.(*atomic.Uint64).Load()&(uint64(1)<<(key%64)) != 0
}
func (b *Bit) Set(key uint64, value bool) {
	group, mask := key/64, uint64(1)<<(key%64)
	var cell *atomic.Uint64
	if value {
		v, _ := b.groups.LoadOrStore(group, new(atomic.Uint64))
		cell = v.(*atomic.Uint64)
	} else {
		v, ok := b.groups.Load(group)
		if !ok {
			return
		}
		cell = v.(*atomic.Uint64)
	}
	for {
		old := cell.Load()
		next := old &^ mask
		if value {
			next = old | mask
		}
		if cell.CompareAndSwap(old, next) {
			return
		}
	}
}
func (b *Bit) Del(key uint64) { b.Set(key, false) }

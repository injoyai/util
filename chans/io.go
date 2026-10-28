package chans

import (
	"errors"
	"io"
	"sync"
)

var _ io.ReadWriteCloser = new(IO)

// NewIO 创建按消息复制写入的内存流，capacity 是消息数。
func NewIO(capacity int) (*IO, error) {
	if capacity < 0 {
		return nil, errors.New("chans: negative capacity")
	}
	return &IO{ch: make(chan []byte, capacity), closeSign: make(chan struct{})}, nil
}

// IO 可并发读写；Close 解除阻塞的写入，随后读取可排空缓冲区。
// 必须通过 NewIO 创建，不可复制；零长度写入不会创建消息。
type IO struct {
	ch        chan []byte
	once      sync.Once
	closeSign chan struct{}
	writeMu   sync.RWMutex
	readMu    sync.Mutex
	readCache []byte
}

func (this *IO) Write(p []byte) (int, error) {
	this.writeMu.RLock()
	defer this.writeMu.RUnlock()
	if this.Closed() {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}
	b := append([]byte(nil), p...)
	select {
	case <-this.closeSign:
		return 0, io.ErrClosedPipe
	case this.ch <- b:
		return len(b), nil
	}
}

// ReadBytes 返回下一条消息；与 Read 混用时先返回上一条消息的剩余字节。
func (this *IO) ReadBytes() ([]byte, error) {
	this.readMu.Lock()
	defer this.readMu.Unlock()
	if len(this.readCache) > 0 {
		bs := this.readCache
		this.readCache = nil
		return bs, nil
	}
	return this.readBytes()
}

func (this *IO) readBytes() ([]byte, error) {
	bs, ok := <-this.ch
	if !ok {
		return nil, io.EOF
	}
	return bs, nil
}

func (this *IO) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	this.readMu.Lock()
	defer this.readMu.Unlock()
	for len(this.readCache) == 0 {
		var err error
		this.readCache, err = this.readBytes()
		if err != nil {
			return 0, err
		}
	}
	n := copy(p, this.readCache)
	this.readCache = this.readCache[n:]
	if len(this.readCache) == 0 {
		this.readCache = nil
	}
	return n, nil
}

func (this *IO) Close() error {
	this.once.Do(func() {
		close(this.closeSign)
		// Release blocked writers before waiting for in-flight writes.
		this.writeMu.Lock()
		close(this.ch)
		this.writeMu.Unlock()
	})
	return nil
}

func (this *IO) Closed() bool {
	select {
	case <-this.closeSign:
		return true
	default:
		return false
	}
}

func (this *IO) Len() int { return len(this.ch) }
func (this *IO) Cap() int { return cap(this.ch) }

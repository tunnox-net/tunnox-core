package utils

import (
	"io"
	"sync"
)

// BufferPool 高效的内存池
// 用于复用不同大小的[]byte，减少GC压力
type BufferPool struct {
	pools map[int]*sync.Pool
	mu    sync.RWMutex
}

// NewBufferPool 创建新的内存池
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pools: make(map[int]*sync.Pool),
	}
}

// Get(size int) []byte 获取指定大小的缓冲区
func (bp *BufferPool) Get(size int) []byte {
	bp.mu.RLock()
	pool, exists := bp.pools[size]
	bp.mu.RUnlock()

	if !exists {
		bp.mu.Lock()
		defer bp.mu.Unlock()

		// 双重检查
		if pool, exists = bp.pools[size]; !exists {
			pool = &sync.Pool{
				New: func() interface{} {
					return make([]byte, size)
				},
			}
			bp.pools[size] = pool
		}
	}

	return pool.Get().([]byte)
}

// Put(buf []byte) 归还缓冲区
func (bp *BufferPool) Put(buf []byte) {
	if buf == nil {
		return
	}

	bp.mu.RLock()
	pool, exists := bp.pools[len(buf)]
	bp.mu.RUnlock()

	if exists {
		// 清空缓冲区内容
		for i := range buf {
			buf[i] = 0
		}
		pool.Put(buf)
	}
}

// BufferManager 缓冲区管理器
// Allocate(size int) []byte 分配缓冲区
// Release(buf []byte) 释放缓冲区
// ReadIntoBuffer(reader io.Reader, size int) ([]byte, error) 读取数据到缓冲区
// GetPool() *BufferPool 获取底层内存池
type BufferManager struct {
	pool *BufferPool
}

// NewBufferManager 创建缓冲区管理器
func NewBufferManager() *BufferManager {
	return &BufferManager{
		pool: NewBufferPool(),
	}
}

// Allocate(size int) []byte 分配缓冲区
func (bm *BufferManager) Allocate(size int) []byte {
	return bm.pool.Get(size)
}

// Release(buf []byte) 释放缓冲区
func (bm *BufferManager) Release(buf []byte) {
	bm.pool.Put(buf)
}

// ReadIntoBuffer(reader io.Reader, size int) ([]byte, error) 读取数据到缓冲区
func (bm *BufferManager) ReadIntoBuffer(reader io.Reader, size int) ([]byte, error) {
	buf := bm.Allocate(size)
	var err error
	defer func() {
		if err != nil {
			bm.Release(buf)
		}
	}()

	totalRead := 0
	for totalRead < size {
		n, readErr := reader.Read(buf[totalRead:])
		totalRead += n

		if readErr != nil {
			err = readErr
			return buf[:totalRead], err
		}

		if n == 0 {
			break
		}
	}

	return buf[:totalRead], nil
}

// GetPool() *BufferPool 获取底层内存池
func (bm *BufferManager) GetPool() *BufferPool {
	return bm.pool
}

// ZeroCopyBuffer 零拷贝缓冲区，避免不必要的内存拷贝
// Data() []byte 获取底层数据（只读）
// Length() int 获取数据长度
// Close() 关闭缓冲区，归还内存
// Copy() []byte 创建数据的副本（当需要修改数据时使用）
type ZeroCopyBuffer struct {
	data   []byte
	pool   *BufferPool
	closed bool
}

// NewZeroCopyBuffer 创建零拷贝缓冲区
func NewZeroCopyBuffer(data []byte, pool *BufferPool) *ZeroCopyBuffer {
	return &ZeroCopyBuffer{
		data: data,
		pool: pool,
	}
}

// Data() []byte 获取底层数据（只读）
func (zcb *ZeroCopyBuffer) Data() []byte {
	return zcb.data
}

// Length() int 获取数据长度
func (zcb *ZeroCopyBuffer) Length() int {
	return len(zcb.data)
}

// Close() 关闭缓冲区，归还内存
func (zcb *ZeroCopyBuffer) Close() {
	if !zcb.closed && zcb.pool != nil {
		zcb.pool.Put(zcb.data)
		zcb.closed = true
	}
}

// Copy() []byte 创建数据的副本（当需要修改数据时使用）
func (zcb *ZeroCopyBuffer) Copy() []byte {
	result := make([]byte, len(zcb.data))
	copy(result, zcb.data)
	return result
}

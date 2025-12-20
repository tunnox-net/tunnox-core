package utils

import (
	"context"
	"fmt"
	"io"
	"sync"
)

const (
	// MaxBufferSize 最大缓冲区大小（16MB），超过此大小直接分配，不放入池中
	MaxBufferSize = 16 * 1024 * 1024
	// BufferSizeAlignment 缓冲区大小对齐（4KB），减少不同大小的 pool 数量
	BufferSizeAlignment = 4 * 1024
)

// BufferPool 内存池，用于管理不同大小的缓冲区
type BufferPool struct {
	pools map[int]*sync.Pool
	mu    sync.RWMutex
	Dispose
}

// NewBufferPool 创建新的内存池
func NewBufferPool(parentCtx context.Context) *BufferPool {
	pool := &BufferPool{
		pools: make(map[int]*sync.Pool),
	}
	pool.SetCtx(parentCtx, nil)
	pool.AddCleanHandler(pool.onClose)
	return pool
}

// onClose 资源释放回调
func (bp *BufferPool) onClose() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	for k := range bp.pools {
		delete(bp.pools, k)
	}
	return nil
}

// alignBufferSize 对齐缓冲区大小，减少不同大小的 pool 数量
func alignBufferSize(size int) int {
	if size <= 0 {
		return BufferSizeAlignment
	}
	if size > MaxBufferSize {
		// 超过最大大小，不对齐，直接返回（不会放入池中）
		return size
	}
	// 对齐到 BufferSizeAlignment 的倍数
	aligned := (size + BufferSizeAlignment - 1) / BufferSizeAlignment * BufferSizeAlignment
	return aligned
}

// Get(size int) []byte 获取指定大小的缓冲区
func (bp *BufferPool) Get(size int) []byte {
	// 超过最大大小，直接分配，不放入池中
	if size > MaxBufferSize {
		return make([]byte, size)
	}

	// 对齐大小，减少不同大小的 pool 数量
	alignedSize := alignBufferSize(size)

	bp.mu.RLock()
	pool, exists := bp.pools[alignedSize]
	bp.mu.RUnlock()

	if !exists {
		bp.mu.Lock()
		defer bp.mu.Unlock()

		// 双重检查
		if pool, exists = bp.pools[alignedSize]; !exists {
			pool = &sync.Pool{
				New: func() interface{} {
					return make([]byte, alignedSize)
				},
			}
			bp.pools[alignedSize] = pool
		}
	}

	buf := pool.Get().([]byte)
	// 确保返回的切片长度正确，但保持底层数组的容量
	// 这样 Put 时可以通过 cap(buf) 正确识别并归还
	if len(buf) != size {
		// 如果长度不匹配，调整长度但保持容量不变
		return buf[:size:cap(buf)]
	}
	return buf
}

// Put(buf []byte) 归还缓冲区
// 🚀 性能优化: 移除清空缓冲区操作（不必要的开销）
func (bp *BufferPool) Put(buf []byte) {
	if buf == nil {
		return
	}

	actualSize := cap(buf)
	if actualSize > MaxBufferSize {
		return
	}

	alignedSize := alignBufferSize(actualSize)

	bp.mu.RLock()
	pool, exists := bp.pools[alignedSize]
	bp.mu.RUnlock()

	if exists {
		// 🚀 直接归还，不清空（调用方负责正确使用）
		pool.Put(buf[:actualSize])
	}
}

// BufferManager 缓冲区管理器，提供高级的缓冲区操作接口
type BufferManager struct {
	pool *BufferPool
	Dispose
}

// NewBufferManager 创建缓冲区管理器
func NewBufferManager(parentCtx context.Context) *BufferManager {
	bm := &BufferManager{
		pool: NewBufferPool(parentCtx),
	}
	bm.SetCtx(parentCtx, nil)
	bm.AddCleanHandler(bm.onClose)
	return bm
}

// onClose 资源释放回调
func (bm *BufferManager) onClose() error {
	if bm.pool != nil {
		result := bm.pool.Close()
		if result.HasErrors() {
			return fmt.Errorf("buffer pool cleanup failed: %v", result.Error())
		}
	}
	return nil
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
// 注意：返回的切片是 buf 的子切片，调用方需要负责释放原始缓冲区
// 如果调用方需要长期持有数据，应该复制数据
func (bm *BufferManager) ReadIntoBuffer(reader io.Reader, size int) ([]byte, error) {
	buf := bm.Allocate(size)
	totalRead := 0

	for totalRead < size {
		n, readErr := reader.Read(buf[totalRead:])
		totalRead += n

		if readErr != nil {
			// 读取错误，释放缓冲区
			bm.Release(buf)
			return nil, readErr
		}

		if n == 0 {
			break
		}
	}

	// 成功读取，返回数据切片（调用方需要负责释放原始缓冲区）
	// 注意：这里返回的是 buf 的子切片，调用方应该复制数据或确保释放原始缓冲区
	return buf[:totalRead], nil
}

// GetPool() *BufferPool 获取底层内存池
func (bm *BufferManager) GetPool() *BufferPool {
	return bm.pool
}

// Close 方法由 utils.Dispose 提供，无需重复实现

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

// Close 方法由 utils.Dispose 提供，无需重复实现

package buffer

import (
	"sync"
)

// BufferPool 缓冲区池接口
type BufferPool interface {
	// Get 获取缓冲区
	Get(size int) []byte

	// Put 归还缓冲区
	Put(buf []byte)

	// GetStats 获取统计信息
	GetStats() *PoolStats
}

// PoolStats 池统计信息
type PoolStats struct {
	TotalAllocated int64 `json:"total_allocated"`
	TotalReturned  int64 `json:"total_returned"`
	CurrentInUse   int64 `json:"current_in_use"`
	PoolSize       int   `json:"pool_size"`
}

// DefaultBufferPool 默认缓冲区池实现
type DefaultBufferPool struct {
	pools map[int]*sync.Pool
	stats *PoolStats
	mutex sync.RWMutex
}

// NewDefaultBufferPool 创建新的默认缓冲区池
func NewDefaultBufferPool() *DefaultBufferPool {
	return &DefaultBufferPool{
		pools: make(map[int]*sync.Pool),
		stats: &PoolStats{},
	}
}

// Get 获取缓冲区
func (bp *DefaultBufferPool) Get(size int) []byte {
	bp.mutex.RLock()
	pool, exists := bp.pools[size]
	bp.mutex.RUnlock()

	if !exists {
		bp.mutex.Lock()
		pool, exists = bp.pools[size]
		if !exists {
			pool = &sync.Pool{
				New: func() interface{} {
					return make([]byte, size)
				},
			}
			bp.pools[size] = pool
		}
		bp.mutex.Unlock()
	}

	buf := pool.Get().([]byte)
	bp.mutex.Lock()
	bp.stats.TotalAllocated++
	bp.stats.CurrentInUse++
	bp.mutex.Unlock()

	return buf
}

// Put 归还缓冲区
func (bp *DefaultBufferPool) Put(buf []byte) {
	if buf == nil {
		return
	}

	size := len(buf)
	bp.mutex.RLock()
	pool, exists := bp.pools[size]
	bp.mutex.RUnlock()

	if exists {
		pool.Put(buf)
		bp.mutex.Lock()
		bp.stats.TotalReturned++
		bp.stats.CurrentInUse--
		bp.mutex.Unlock()
	}
}

// GetStats 获取统计信息
func (bp *DefaultBufferPool) GetStats() *PoolStats {
	bp.mutex.RLock()
	defer bp.mutex.RUnlock()

	stats := *bp.stats
	stats.PoolSize = len(bp.pools)
	return &stats
}

// BufferManager 缓冲区管理器
type BufferManager struct {
	pool BufferPool
}

// NewBufferManager 创建新的缓冲区管理器
func NewBufferManager() *BufferManager {
	return &BufferManager{
		pool: NewDefaultBufferPool(),
	}
}

// AllocateBuffer 分配缓冲区
func (bm *BufferManager) AllocateBuffer(size int) []byte {
	return bm.pool.Get(size)
}

// ReleaseBuffer 释放缓冲区
func (bm *BufferManager) ReleaseBuffer(buf []byte) {
	bm.pool.Put(buf)
}

// GetPoolStats 获取池统计信息
func (bm *BufferManager) GetPoolStats() *PoolStats {
	return bm.pool.GetStats()
}

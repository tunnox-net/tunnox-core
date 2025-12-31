package session

import (
	"time"

	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/protocol/session/connstate"
)

// ============================================================================
// 连接状态分布式存储（跨节点查询）
// 类型别名 - 实现已移至 connstate 子包
// ============================================================================

// ConnectionStateInfo 连接状态信息（用于跨节点查询）
// 类型别名，保持 API 兼容性
type ConnectionStateInfo = connstate.Info

// ConnectionStateStore 连接状态分布式存储
// 类型别名，保持 API 兼容性
type ConnectionStateStore = connstate.Store

// NewConnectionStateStore 创建连接状态存储
// 工厂函数，保持 API 兼容性
func NewConnectionStateStore(storage storage.Storage, nodeID string, ttl time.Duration) *ConnectionStateStore {
	return connstate.NewStore(storage, nodeID, ttl)
}

// ErrConnectionNotFound 连接未找到错误
var ErrConnectionNotFound = connstate.ErrConnectionNotFound

// ErrConnectionExpired 连接状态已过期错误
var ErrConnectionExpired = connstate.ErrConnectionExpired

package server

import (
	"sync"
	"sync/atomic"
	"time"
	"tunnox-core/internal/bridge"
	"tunnox-core/internal/protocol/session"
)

// Tunnel 隧道实例
type Tunnel struct {
	TunnelID       string
	MappingID      string
	SourceClientID int64
	TargetClientID int64
	SourceConn     *session.ClientConnection // 到源客户端的连接（仅用于获取 Reader/Writer）
	TargetConn     interface{}               // 本地: *ClientConnection, 跨节点: *ForwardSession
	IsLocal        bool
	CreatedAt      time.Time
	LastActiveAt   time.Time
	BytesSent      uint64
	BytesReceived  uint64
	
	// ✅ 用于双向 Copy 的控制
	copyStarted    bool          // 是否已启动 Copy
	copyDone       chan struct{} // Copy 完成信号
	
	mu sync.RWMutex
}

// UpdateActivity 更新活跃时间
func (t *Tunnel) UpdateActivity() {
	t.mu.Lock()
	t.LastActiveAt = time.Now()
	t.mu.Unlock()
}

// UpdateStats 更新统计信息
func (t *Tunnel) UpdateStats(sent, received uint64) {
	atomic.AddUint64(&t.BytesSent, sent)
	atomic.AddUint64(&t.BytesReceived, received)
	
	t.UpdateActivity()
}

// GetStats 获取统计信息
func (t *Tunnel) GetStats() (sent, received uint64, lastActive time.Time) {
	sent = atomic.LoadUint64(&t.BytesSent)
	received = atomic.LoadUint64(&t.BytesReceived)
	
	t.mu.RLock()
	lastActive = t.LastActiveAt
	t.mu.RUnlock()
	
	return
}

// Close 关闭隧道
func (t *Tunnel) Close() error {
	if !t.IsLocal {
		if session, ok := t.TargetConn.(*bridge.ForwardSession); ok {
			return session.Close()
		}
	}
	return nil
}


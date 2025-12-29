package session

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"

	"golang.org/x/time/rate"
)

// ============================================================================
// 接口定义
// ============================================================================

// TunnelBridgeAccessor 隧道桥接访问器接口（用于API层和跨包访问）
// 避免循环依赖，提供最小化的访问接口
type TunnelBridgeAccessor interface {
	GetTunnelID() string
	GetSourceConnectionID() string
	GetTargetConnectionID() string
	GetMappingID() string
	GetClientID() int64
	IsActive() bool
	Close() error
}

// ============================================================================
// 结构体定义
// ============================================================================

// TunnelBridge 隧道桥接器
// 负责在两个隧道连接之间进行数据桥接（源端客户端 <-> 目标端客户端）
//
// 商业特性支持：
// - 流量统计：统计实际传输的字节数（已加密/压缩后）
// - 带宽限制：使用 Token Bucket 限制传输速率
// - 连接监控：记录活跃隧道和传输状态
type TunnelBridge struct {
	*dispose.ManagerBase

	tunnelID  string
	mappingID string // 映射ID（用于流量统计）

	// 统一接口（推荐使用）
	sourceTunnelConn TunnelConnectionInterface // 源端隧道连接（统一接口）
	targetTunnelConn TunnelConnectionInterface // 目标端隧道连接（统一接口）
	tunnelConnMu     sync.RWMutex              // 保护隧道连接的读写

	// 向后兼容（逐步迁移）
	sourceConn      net.Conn // 源端客户端的隧道连接
	sourceStream    stream.PackageStreamer
	sourceForwarder DataForwarder // 源端数据转发器（接口抽象）
	sourceConnMu    sync.RWMutex  // 保护 sourceConn 的读写
	targetConn      net.Conn      // 目标端客户端的隧道连接
	targetStream    stream.PackageStreamer
	targetForwarder DataForwarder // 目标端数据转发器（接口抽象）

	ready chan struct{} // 用于通知目标端连接已建立

	// 跨节点支持
	crossNodeConn   *CrossNodeConn // 跨节点连接（如果有）
	crossNodeConnMu sync.RWMutex   // 保护跨节点连接

	// 商业特性
	rateLimiter          *rate.Limiter   // 带宽限制器
	bytesSent            atomic.Int64    // 源端→目标端字节数
	bytesReceived        atomic.Int64    // 目标端→源端字节数
	lastReportedSent     atomic.Int64    // 上次上报的发送字节数
	lastReportedReceived atomic.Int64    // 上次上报的接收字节数
	cloudControl         CloudControlAPI // 用于上报流量统计
}

// TunnelBridgeConfig 隧道桥接器配置
type TunnelBridgeConfig struct {
	TunnelID  string
	MappingID string

	// 统一接口（推荐）
	SourceTunnelConn TunnelConnectionInterface

	// 向后兼容
	SourceConn   net.Conn
	SourceStream stream.PackageStreamer

	BandwidthLimit int64           // 字节/秒，0表示不限制
	CloudControl   CloudControlAPI // 用于上报流量统计（可选）
}

// ============================================================================
// 构造函数和生命周期
// ============================================================================

// NewTunnelBridge 创建隧道桥接器
func NewTunnelBridge(parentCtx context.Context, config *TunnelBridgeConfig) *TunnelBridge {
	bridge := &TunnelBridge{
		ManagerBase:  dispose.NewManager(fmt.Sprintf("TunnelBridge-%s", config.TunnelID), parentCtx),
		tunnelID:     config.TunnelID,
		mappingID:    config.MappingID,
		cloudControl: config.CloudControl,
		ready:        make(chan struct{}),
	}

	// 优先使用统一接口
	if config.SourceTunnelConn != nil {
		bridge.sourceTunnelConn = config.SourceTunnelConn
		bridge.sourceConn = config.SourceTunnelConn.GetNetConn()
		bridge.sourceStream = config.SourceTunnelConn.GetStream()
	} else {
		// 向后兼容：从 net.Conn + stream 创建统一接口
		bridge.sourceConn = config.SourceConn
		bridge.sourceStream = config.SourceStream
		if config.SourceConn != nil || config.SourceStream != nil {
			// 提取连接信息
			connID := ""
			if config.SourceConn != nil {
				connID = config.SourceConn.RemoteAddr().String()
			}
			clientID := extractClientID(config.SourceStream, config.SourceConn)
			bridge.sourceTunnelConn = CreateTunnelConnection(
				connID,
				config.SourceConn,
				config.SourceStream,
				clientID,
				config.MappingID,
				config.TunnelID,
			)
		}
	}

	// 创建数据转发器
	bridge.sourceForwarder = createDataForwarder(bridge.sourceConn, bridge.sourceStream)

	// 配置带宽限制
	if config.BandwidthLimit > 0 {
		bridge.rateLimiter = rate.NewLimiter(rate.Limit(config.BandwidthLimit), int(config.BandwidthLimit*2))
		corelog.Infof("TunnelBridge[%s]: bandwidth limit set to %d bytes/sec", config.TunnelID, config.BandwidthLimit)
	}

	// 注册清理处理器
	bridge.AddCleanHandler(func() error {
		corelog.Infof("TunnelBridge[%s]: cleaning up resources", config.TunnelID)

		// 上报最终流量统计
		bridge.reportTrafficStats()

		var errs []error

		// 释放跨节点连接（归还到池）
		bridge.ReleaseCrossNodeConnection()

		// 关闭统一接口连接
		if bridge.sourceTunnelConn != nil {
			if err := bridge.sourceTunnelConn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("source tunnel conn close error: %w", err))
			}
		}
		if bridge.targetTunnelConn != nil {
			if err := bridge.targetTunnelConn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("target tunnel conn close error: %w", err))
			}
		}

		// 向后兼容：关闭旧接口
		if bridge.sourceStream != nil && bridge.sourceTunnelConn == nil {
			bridge.sourceStream.Close()
		}
		if bridge.targetStream != nil && bridge.targetTunnelConn == nil {
			bridge.targetStream.Close()
		}
		if bridge.sourceConn != nil && bridge.sourceTunnelConn == nil {
			if err := bridge.sourceConn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("source conn close error: %w", err))
			}
		}
		if bridge.targetConn != nil && bridge.targetTunnelConn == nil {
			if err := bridge.targetConn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("target conn close error: %w", err))
			}
		}

		if len(errs) > 0 {
			return fmt.Errorf("tunnel bridge cleanup errors: %v", errs)
		}
		return nil
	})

	return bridge
}

// Close 关闭桥接
func (b *TunnelBridge) Close() error {
	b.ManagerBase.Close()
	return nil
}

// ============================================================================
// 访问器方法 (实现 TunnelBridgeAccessor 接口)
// ============================================================================

// GetTunnelID 获取隧道ID
func (b *TunnelBridge) GetTunnelID() string {
	if b == nil {
		return ""
	}
	return b.tunnelID
}

// GetSourceConnectionID 获取源连接ID
func (b *TunnelBridge) GetSourceConnectionID() string {
	if b == nil {
		return ""
	}
	b.tunnelConnMu.RLock()
	defer b.tunnelConnMu.RUnlock()
	if b.sourceTunnelConn != nil {
		return b.sourceTunnelConn.GetConnectionID()
	}
	return ""
}

// GetTargetConnectionID 获取目标连接ID
func (b *TunnelBridge) GetTargetConnectionID() string {
	if b == nil {
		return ""
	}
	b.tunnelConnMu.RLock()
	defer b.tunnelConnMu.RUnlock()
	if b.targetTunnelConn != nil {
		return b.targetTunnelConn.GetConnectionID()
	}
	return ""
}

// GetMappingID 获取映射ID
func (b *TunnelBridge) GetMappingID() string {
	if b == nil {
		return ""
	}
	return b.mappingID
}

// GetClientID 获取客户端ID
func (b *TunnelBridge) GetClientID() int64 {
	if b == nil {
		return 0
	}
	b.tunnelConnMu.RLock()
	defer b.tunnelConnMu.RUnlock()
	if b.sourceTunnelConn != nil {
		return b.sourceTunnelConn.GetClientID()
	}
	return 0
}

// IsActive 检查桥接是否活跃
func (b *TunnelBridge) IsActive() bool {
	if b == nil {
		return false
	}
	return !b.IsClosed()
}

// ============================================================================
// 连接管理
// ============================================================================

// SetTargetConnection 设置目标端连接（统一接口）
func (b *TunnelBridge) SetTargetConnection(conn TunnelConnectionInterface) {
	b.tunnelConnMu.Lock()
	b.targetTunnelConn = conn
	if conn != nil {
		b.targetConn = conn.GetNetConn()
		b.targetStream = conn.GetStream()
		b.targetForwarder = createDataForwarder(b.targetConn, b.targetStream)
	}
	b.tunnelConnMu.Unlock()
	close(b.ready)
}

// SetTargetConnectionLegacy 设置目标端连接（向后兼容）
func (b *TunnelBridge) SetTargetConnectionLegacy(targetConn net.Conn, targetStream stream.PackageStreamer) {
	b.targetConn = targetConn
	b.targetStream = targetStream
	b.targetForwarder = createDataForwarder(targetConn, targetStream)

	// 创建统一接口
	if targetConn != nil || targetStream != nil {
		connID := ""
		if targetConn != nil {
			connID = targetConn.RemoteAddr().String()
		}
		clientID := extractClientID(targetStream, targetConn)
		b.tunnelConnMu.Lock()
		b.targetTunnelConn = CreateTunnelConnection(
			connID,
			targetConn,
			targetStream,
			clientID,
			b.mappingID,
			b.tunnelID,
		)
		b.tunnelConnMu.Unlock()
	}

	close(b.ready)
}

// SetSourceConnection 设置源端连接（统一接口）
func (b *TunnelBridge) SetSourceConnection(conn TunnelConnectionInterface) {
	b.tunnelConnMu.Lock()
	b.sourceTunnelConn = conn
	if conn != nil {
		b.sourceConn = conn.GetNetConn()
		b.sourceStream = conn.GetStream()
		b.sourceForwarder = createDataForwarder(b.sourceConn, b.sourceStream)
	} else {
		b.sourceForwarder = nil
	}
	b.tunnelConnMu.Unlock()
}

// SetSourceConnectionLegacy 设置源端连接（向后兼容）
func (b *TunnelBridge) SetSourceConnectionLegacy(sourceConn net.Conn, sourceStream stream.PackageStreamer) {
	b.sourceConnMu.Lock()
	b.sourceConn = sourceConn
	b.sourceForwarder = createDataForwarder(sourceConn, sourceStream)
	b.sourceConnMu.Unlock()
	if sourceStream != nil {
		b.sourceStream = sourceStream
	}

	// 创建统一接口
	if sourceConn != nil || sourceStream != nil {
		connID := ""
		if sourceConn != nil {
			connID = sourceConn.RemoteAddr().String()
		}
		clientID := extractClientID(sourceStream, sourceConn)
		b.tunnelConnMu.Lock()
		b.sourceTunnelConn = CreateTunnelConnection(
			connID,
			sourceConn,
			sourceStream,
			clientID,
			b.mappingID,
			b.tunnelID,
		)
		b.tunnelConnMu.Unlock()
	}
}

// getSourceConn 获取源端连接（线程安全）
func (b *TunnelBridge) getSourceConn() net.Conn {
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceConn
}

// getSourceForwarder 获取源端数据转发器（线程安全）
func (b *TunnelBridge) getSourceForwarder() DataForwarder {
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceForwarder
}

// WaitForTarget 等待目标端连接就绪
func (b *TunnelBridge) WaitForTarget(timeout time.Duration) error {
	select {
	case <-b.ready:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for target connection")
	case <-b.Ctx().Done():
		return b.Ctx().Err()
	}
}

// IsTargetReady 检查目标端是否就绪
func (b *TunnelBridge) IsTargetReady() bool {
	select {
	case <-b.ready:
		return true
	default:
		return false
	}
}

// NotifyTargetReady 通知目标端就绪（用于跨节点场景）
func (b *TunnelBridge) NotifyTargetReady() {
	select {
	case <-b.ready:
		// 已经关闭，忽略
	default:
		close(b.ready)
	}
}

// ============================================================================
// 跨节点连接管理
// ============================================================================

// SetCrossNodeConnection 设置跨节点连接
func (b *TunnelBridge) SetCrossNodeConnection(conn *CrossNodeConn) {
	b.crossNodeConnMu.Lock()
	b.crossNodeConn = conn
	b.crossNodeConnMu.Unlock()
}

// GetCrossNodeConnection 获取跨节点连接
func (b *TunnelBridge) GetCrossNodeConnection() *CrossNodeConn {
	b.crossNodeConnMu.RLock()
	defer b.crossNodeConnMu.RUnlock()
	return b.crossNodeConn
}

// ReleaseCrossNodeConnection 释放跨节点连接
// 只清理 Bridge 中的引用，连接的生命周期由数据转发函数管理
func (b *TunnelBridge) ReleaseCrossNodeConnection() {
	b.crossNodeConnMu.Lock()
	b.crossNodeConn = nil // 只清理引用，不关闭连接
	b.crossNodeConnMu.Unlock()
}

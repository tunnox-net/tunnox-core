package session

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/stream"

	"golang.org/x/time/rate"
)

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

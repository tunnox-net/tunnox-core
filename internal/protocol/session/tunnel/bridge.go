// Package tunnel 提供隧道桥接和路由功能
package tunnel

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"

	"golang.org/x/time/rate"
)

// ============================================================================
// 接口定义
// ============================================================================

// BridgeAccessor 隧道桥接访问器接口（用于API层和跨包访问）
// 避免循环依赖，提供最小化的访问接口
type BridgeAccessor interface {
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

// Bridge 隧道桥接器
// 负责在两个隧道连接之间进行数据桥接（源端客户端 <-> 目标端客户端）
//
// 商业特性支持：
// - 流量统计：统计实际传输的字节数（已加密/压缩后）
// - 带宽限制：使用 Token Bucket 限制传输速率
// - 连接监控：记录活跃隧道和传输状态
type Bridge struct {
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
	crossNodeConn   CrossNodeConnInterface // 跨节点连接（如果有）
	crossNodeConnMu sync.RWMutex           // 保护跨节点连接

	// 商业特性
	rateLimiter          *rate.Limiter   // 带宽限制器
	bytesSent            atomic.Int64    // 源端→目标端字节数
	bytesReceived        atomic.Int64    // 目标端→源端字节数
	lastReportedSent     atomic.Int64    // 上次上报的发送字节数
	lastReportedReceived atomic.Int64    // 上次上报的接收字节数
	cloudControl         CloudControlAPI // 用于上报流量统计

	// 配额管理
	trafficMeter  *stream.TrafficMeter  // 流量计量器（每个 Bridge 一个）
	quotaEnforcer *stream.QuotaEnforcer // 配额执行器（全局共享）
	clientID      int64                 // 客户端ID（用于配额检查）
}

// BridgeConfig 隧道桥接器配置
type BridgeConfig struct {
	TunnelID  string
	MappingID string
	ClientID  int64

	// 统一接口（推荐）
	SourceTunnelConn TunnelConnectionInterface

	// 向后兼容
	SourceConn   net.Conn
	SourceStream stream.PackageStreamer

	BandwidthLimit int64           // 字节/秒，0表示不限制
	CloudControl   CloudControlAPI // 用于上报流量统计（可选）

	// 配额管理
	QuotaEnforcer *stream.QuotaEnforcer
}

// ============================================================================
// 隧道连接工厂
// ============================================================================

// TunnelConnectionFactory 隧道连接工厂函数类型
type TunnelConnectionFactory func(
	connID string,
	conn net.Conn,
	stream stream.PackageStreamer,
	clientID int64,
	mappingID string,
	tunnelID string,
) TunnelConnectionInterface

// 全局隧道连接工厂（由 session 包设置）
var tunnelConnFactory TunnelConnectionFactory

// SetTunnelConnectionFactory 设置隧道连接工厂
func SetTunnelConnectionFactory(factory TunnelConnectionFactory) {
	tunnelConnFactory = factory
}

// createTunnelConnection 创建隧道连接（使用工厂）
func createTunnelConnection(
	connID string,
	conn net.Conn,
	stream stream.PackageStreamer,
	clientID int64,
	mappingID string,
	tunnelID string,
) TunnelConnectionInterface {
	if tunnelConnFactory != nil {
		return tunnelConnFactory(connID, conn, stream, clientID, mappingID, tunnelID)
	}
	return nil
}

// ============================================================================
// 构造函数和生命周期
// ============================================================================

// NewBridge 创建隧道桥接器
func NewBridge(parentCtx context.Context, config *BridgeConfig) *Bridge {
	corelog.Infof("TunnelBridge[%s]: creating bridge - mappingID=%s, clientID=%d, cloudControl=%v, quotaEnforcer=%v",
		config.TunnelID, config.MappingID, config.ClientID, config.CloudControl != nil, config.QuotaEnforcer != nil)

	bridge := &Bridge{
		ManagerBase:   dispose.NewManager("TunnelBridge-"+config.TunnelID, parentCtx),
		tunnelID:      config.TunnelID,
		mappingID:     config.MappingID,
		clientID:      config.ClientID,
		cloudControl:  config.CloudControl,
		quotaEnforcer: config.QuotaEnforcer,
		ready:         make(chan struct{}),
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
			bridge.sourceTunnelConn = createTunnelConnection(
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
	bridge.sourceForwarder = CreateDataForwarder(bridge.sourceConn, bridge.sourceStream)

	// 配置带宽限制
	if config.BandwidthLimit > 0 {
		bridge.rateLimiter = rate.NewLimiter(rate.Limit(config.BandwidthLimit), int(config.BandwidthLimit*2))
		corelog.Infof("TunnelBridge[%s]: bandwidth limit set to %d bytes/sec", config.TunnelID, config.BandwidthLimit)
	}

	// 初始化流量计量器（如果有配额执行器）
	if config.QuotaEnforcer != nil && config.ClientID > 0 {
		meterConfig := &stream.TrafficMeterConfig{
			UserID:         config.ClientID,
			MappingID:      config.MappingID,
			TunnelID:       config.TunnelID,
			ReportInterval: 30 * time.Second,
			BandwidthLimit: config.BandwidthLimit,
			Logger:         corelog.Default(),
		}
		bridge.trafficMeter = stream.NewTrafficMeter(meterConfig, bridge.Ctx())
		config.QuotaEnforcer.RegisterMeter(bridge.trafficMeter)
		corelog.Infof("TunnelBridge[%s]: traffic meter registered for client=%d", config.TunnelID, config.ClientID)
	}

	// 注册清理处理器
	bridge.AddCleanHandler(func() error {
		return bridge.cleanup(config.TunnelID)
	})

	// 启动定期流量上报
	bridge.StartPeriodicTrafficReport()

	return bridge
}

// cleanup 清理桥接资源（连接关闭由 Close() 方法处理）
func (b *Bridge) cleanup(tunnelID string) error {
	corelog.Infof("TunnelBridge[%s]: cleaning up resources", tunnelID)

	b.reportTrafficStats()

	if b.quotaEnforcer != nil && b.mappingID != "" {
		b.quotaEnforcer.UnregisterMeter(b.mappingID)
	}

	b.ReleaseCrossNodeConnection()

	return nil
}

// Close 关闭桥接
// 重要：必须先关闭底层连接，使阻塞在 Read() 上的 goroutine 能够退出
// 否则 CopyWithControl 中的 src.Read(buf) 会一直阻塞，导致 goroutine 泄漏
func (b *Bridge) Close() error {
	// 1. 先关闭底层连接，使 CopyWithControl 中的 Read() 返回错误
	// 这样数据转发 goroutine 才能退出
	b.tunnelConnMu.Lock()
	if b.sourceTunnelConn != nil {
		b.sourceTunnelConn.Close()
	}
	if b.targetTunnelConn != nil {
		b.targetTunnelConn.Close()
	}
	// 向后兼容：关闭旧接口连接
	if b.sourceConn != nil && b.sourceTunnelConn == nil {
		b.sourceConn.Close()
	}
	if b.targetConn != nil && b.targetTunnelConn == nil {
		b.targetConn.Close()
	}
	b.tunnelConnMu.Unlock()

	// 2. 然后取消 context 并运行其他清理处理器
	// cleanup() 中的连接关闭会被忽略（已经关闭）
	b.ManagerBase.Close()
	return nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// extractClientID 从 stream 或 conn 中提取 clientID
func extractClientID(s stream.PackageStreamer, conn net.Conn) int64 {
	if s != nil {
		if streamWithClientID, ok := s.(interface{ GetClientID() int64 }); ok {
			return streamWithClientID.GetClientID()
		}
	}
	return 0
}

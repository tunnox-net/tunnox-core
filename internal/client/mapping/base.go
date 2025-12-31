package mapping

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"tunnox-core/internal/client/tunnel"
	"tunnox-core/internal/config"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream/transform"
	"tunnox-core/internal/utils/iocopy"

	"golang.org/x/time/rate"
)

// BaseMappingHandler 基础映射处理器
// 提供所有协议通用的逻辑，协议特定部分委托给MappingAdapter
type BaseMappingHandler struct {
	*dispose.ManagerBase

	adapter       MappingAdapter              // 协议适配器（多态）
	client        ClientInterface             // 客户端接口
	config        config.MappingConfig        // 映射配置
	transformer   transform.StreamTransformer // 加密压缩转换器
	tunnelManager tunnel.TunnelManager        // 隧道管理器（Listen角色）

	// 商业化控制
	rateLimiter       *rate.Limiter // 速率限制器（Token Bucket）
	activeConnCount   atomic.Int32  // 当前活跃连接数
	trafficStats      *TrafficStats // 流量统计
	statsReportTicker *time.Ticker  // 统计上报定时器
}

// NewBaseMappingHandler 创建基础映射处理器
func NewBaseMappingHandler(
	client ClientInterface,
	config config.MappingConfig,
	adapter MappingAdapter,
) *BaseMappingHandler {
	handler := &BaseMappingHandler{
		ManagerBase: dispose.NewManager(
			fmt.Sprintf("MappingHandler-%s", config.MappingID),
			client.GetContext(),
		),
		adapter:      adapter,
		client:       client,
		config:       config,
		trafficStats: &TrafficStats{},
	}

	// 创建隧道管理器（Listen角色）
	handler.tunnelManager = tunnel.NewTunnelManager(handler.Ctx(), tunnel.TunnelRoleListen)

	// 创建速率限制器（如果配置了带宽限制）
	if config.BandwidthLimit > 0 {
		handler.rateLimiter = rate.NewLimiter(
			rate.Limit(config.BandwidthLimit), // bytes/s
			int(config.BandwidthLimit*2),      // burst size (2x)
		)
		corelog.Debugf("BaseMappingHandler[%s]: rate limiter enabled, limit=%d bytes/s",
			config.MappingID, config.BandwidthLimit)
	}

	// 启动流量统计上报（每30秒）
	handler.statsReportTicker = time.NewTicker(30 * time.Second)
	go handler.reportStatsLoop()

	// 注册清理处理器
	handler.AddCleanHandler(func() error {
		corelog.Infof("BaseMappingHandler[%s]: cleaning up resources", config.MappingID)

		// 停止统计上报
		if handler.statsReportTicker != nil {
			handler.statsReportTicker.Stop()
		}

		// 最后一次上报流量统计
		handler.reportStats()

		// 关闭隧道管理器
		if handler.tunnelManager != nil {
			handler.tunnelManager.Close()
		}

		// 关闭协议适配器
		return adapter.Close()
	})

	return handler
}

// Start 启动映射处理器
func (h *BaseMappingHandler) Start() error {
	// 1. 创建Transformer（公共）
	if err := h.createTransformer(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to create transformer")
	}

	// 2. 启动监听（委托给adapter）
	if err := h.adapter.StartListener(h.config); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to start listener")
	}

	corelog.Infof("BaseMappingHandler: %s mapping started on port %d",
		h.adapter.GetProtocol(), h.config.LocalPort)

	// 3. 启动接受循环（公共）
	go h.acceptLoop()

	return nil
}

// acceptLoop 接受连接循环
func (h *BaseMappingHandler) acceptLoop() {
	mappingID := h.config.MappingID
	corelog.Infof("BaseMappingHandler[%s]: acceptLoop started", mappingID)
	defer corelog.Infof("BaseMappingHandler[%s]: acceptLoop exited", mappingID)

	for {
		// 检查 context 是否已取消
		select {
		case <-h.Ctx().Done():
			return
		default:
		}

		// 接受连接（委托给adapter，带超时）
		localConn, err := h.adapter.Accept()

		if err != nil {
			// 先检查 context 是否已取消
			if h.Ctx().Err() != nil {
				return
			}

			// 检查是否是超时错误（正常情况，继续等待）
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				continue
			}

			// 其他错误，记录并短暂等待后重试
			corelog.Errorf("BaseMappingHandler[%s]: accept error: %v", mappingID, err)
			time.Sleep(100 * time.Millisecond) // 避免错误循环
			continue
		}

		corelog.Infof("BaseMappingHandler[%s]: new connection accepted", mappingID)
		// 处理连接（公共）
		go h.handleConnection(localConn)
	}
}

// handleConnection 处理单个连接
func (h *BaseMappingHandler) handleConnection(localConn io.ReadWriteCloser) {
	corelog.Infof("BaseMappingHandler[%s]: new connection received", h.config.MappingID)

	// 1. 配额检查：连接数限制
	if err := h.checkConnectionQuota(); err != nil {
		corelog.Warnf("BaseMappingHandler[%s]: quota check failed: %v", h.config.MappingID, err)
		localConn.Close()
		return
	}

	// 增加活跃连接计数
	currentCount := h.activeConnCount.Add(1)
	defer h.activeConnCount.Add(-1)

	corelog.Debugf("BaseMappingHandler[%s]: active connections: %d", h.config.MappingID, currentCount)

	// 2. 连接预处理（委托给adapter）
	if err := h.adapter.PrepareConnection(localConn); err != nil {
		corelog.Errorf("BaseMappingHandler[%s]: prepare connection failed: %v", h.config.MappingID, err)
		localConn.Close()
		return
	}

	// 3. 配额检查：流量限制
	if err := h.client.CheckMappingQuota(h.config.MappingID); err != nil {
		corelog.Warnf("BaseMappingHandler[%s]: mapping quota exceeded: %v", h.config.MappingID, err)
		localConn.Close()
		return
	}

	// 4. 生成隧道ID并建立隧道连接
	tunnelID := h.generateTunnelID()
	corelog.Infof("BaseMappingHandler[%s]: dialing tunnel %s", h.config.MappingID, tunnelID)

	tunnelConn, tunnelStream, err := h.client.DialTunnel(
		tunnelID,
		h.config.MappingID,
		h.config.SecretKey,
	)
	if err != nil {
		corelog.Errorf("BaseMappingHandler[%s]: dial tunnel failed: %v", h.config.MappingID, err)
		localConn.Close()
		return
	}

	corelog.Infof("BaseMappingHandler[%s]: tunnel %s established", h.config.MappingID, tunnelID)

	// 5. 获取隧道 Reader/Writer
	tunnelReader := tunnelStream.GetReader()
	tunnelWriter := tunnelStream.GetWriter()

	// 如果 GetReader/GetWriter 返回 nil，尝试使用 tunnelConn
	if tunnelReader == nil {
		if tunnelConn != nil {
			if reader, ok := tunnelConn.(io.Reader); ok && reader != nil {
				tunnelReader = reader
			} else {
				corelog.Errorf("BaseMappingHandler[%s]: tunnelConn does not implement io.Reader", h.config.MappingID)
				tunnelConn.Close()
				localConn.Close()
				return
			}
		} else {
			corelog.Errorf("BaseMappingHandler[%s]: tunnelConn is nil and GetReader() returned nil", h.config.MappingID)
			localConn.Close()
			return
		}
	}
	if tunnelWriter == nil {
		if tunnelConn != nil {
			if writer, ok := tunnelConn.(io.Writer); ok && writer != nil {
				tunnelWriter = writer
			} else {
				corelog.Errorf("BaseMappingHandler[%s]: tunnelConn does not implement io.Writer", h.config.MappingID)
				tunnelConn.Close()
				localConn.Close()
				return
			}
		} else {
			corelog.Errorf("BaseMappingHandler[%s]: tunnelConn is nil and GetWriter() returned nil", h.config.MappingID)
			localConn.Close()
			return
		}
	}

	// 6. 包装隧道连接成 ReadWriteCloser
	tunnelCloser := func() error {
		tunnelStream.Close()
		if tunnelConn != nil {
			tunnelConn.Close()
		}
		return nil
	}

	tunnelRWC, err := iocopy.NewReadWriteCloser(tunnelReader, tunnelWriter, tunnelCloser)
	if err != nil {
		corelog.Errorf("BaseMappingHandler[%s]: failed to create tunnel ReadWriteCloser: %v", h.config.MappingID, err)
		tunnelCloser()
		localConn.Close()
		return
	}

	// 7. 创建新的 Tunnel 结构
	var tun *tunnel.Tunnel
	tun = tunnel.NewTunnel(&tunnel.TunnelConfig{
		ID:           tunnelID,
		MappingID:    h.config.MappingID,
		Role:         tunnel.TunnelRoleListen,
		Protocol:     h.adapter.GetProtocol(), // 传递协议类型，UDP 使用特殊处理
		LocalConn:    localConn,
		TunnelConn:   tunnelConn,
		TunnelRWC:    tunnelRWC,
		TargetClient: h.config.TargetClientID,
		Manager:      h.tunnelManager,
		Client:       h,
		OnClosed: func(reason tunnel.CloseReason, err error) {
			// 更新流量统计
			stats := tun.GetStats()
			h.trafficStats.BytesSent.Add(stats.BytesSent)
			h.trafficStats.BytesReceived.Add(stats.BytesRecv)
			h.trafficStats.ConnectionCount.Add(1)

			corelog.Infof("BaseMappingHandler[%s]: tunnel %s closed, reason=%s, sent=%d, recv=%d",
				h.config.MappingID, tunnelID, reason, stats.BytesSent, stats.BytesRecv)
		},
	})

	// 8. 注册到 TunnelManager
	if err := h.tunnelManager.RegisterTunnel(tun); err != nil {
		corelog.Errorf("BaseMappingHandler[%s]: failed to register tunnel: %v", h.config.MappingID, err)
		localConn.Close()
		tunnelRWC.Close()
		return
	}

	// 9. 启动 Tunnel（自动管理生命周期）
	if err := tun.Start(); err != nil {
		corelog.Errorf("BaseMappingHandler[%s]: failed to start tunnel: %v", h.config.MappingID, err)
		h.tunnelManager.UnregisterTunnel(tunnelID)
		localConn.Close()
		tunnelRWC.Close()
		return
	}

	corelog.Infof("BaseMappingHandler[%s]: tunnel %s started successfully", h.config.MappingID, tunnelID)

	// Tunnel会在独立的goroutine中管理自己的生命周期，handleConnection可以立即返回
	// 注意：不要在这里等待Tunnel完成，否则可能会影响accept循环
}

// Stop 停止映射处理器
func (h *BaseMappingHandler) Stop() {
	corelog.Infof("BaseMappingHandler[%s]: stopping", h.config.MappingID)
	h.Close()
}

// GetMappingID 获取映射ID
func (h *BaseMappingHandler) GetMappingID() string {
	return h.config.MappingID
}

// GetProtocol 获取协议名称
func (h *BaseMappingHandler) GetProtocol() string {
	return h.adapter.GetProtocol()
}

// GetConfig 获取映射配置
func (h *BaseMappingHandler) GetConfig() config.MappingConfig {
	return h.config
}

// GetContext 获取上下文
func (h *BaseMappingHandler) GetContext() context.Context {
	return h.Ctx()
}

// SendTunnelCloseNotify 发送隧道关闭通知给对端（实现 tunnel.ClientInterface）
func (h *BaseMappingHandler) SendTunnelCloseNotify(targetClientID int64, tunnelID, mappingID, reason string) error {
	return h.client.SendTunnelCloseNotify(targetClientID, tunnelID, mappingID, reason)
}

// GetTunnelManager 获取隧道管理器（用于注册到 NotificationDispatcher）
func (h *BaseMappingHandler) GetTunnelManager() tunnel.TunnelManager {
	return h.tunnelManager
}

package mapping

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"tunnox-core/internal/config"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/stream/transform"
	"tunnox-core/internal/utils"

	"golang.org/x/time/rate"
)

// BaseMappingHandler 基础映射处理器
// 提供所有协议通用的逻辑，协议特定部分委托给MappingAdapter
type BaseMappingHandler struct {
	*dispose.ManagerBase

	adapter     MappingAdapter              // 协议适配器（多态）
	client      ClientInterface             // 客户端接口
	config      config.MappingConfig        // 映射配置
	transformer transform.StreamTransformer // 加密压缩转换器

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

	// 创建速率限制器（如果配置了带宽限制）
	if config.BandwidthLimit > 0 {
		handler.rateLimiter = rate.NewLimiter(
			rate.Limit(config.BandwidthLimit), // bytes/s
			int(config.BandwidthLimit*2),      // burst size (2x)
		)
		utils.Debugf("BaseMappingHandler[%s]: rate limiter enabled, limit=%d bytes/s",
			config.MappingID, config.BandwidthLimit)
	}

	// 启动流量统计上报（每30秒）
	handler.statsReportTicker = time.NewTicker(30 * time.Second)
	go handler.reportStatsLoop()

	// 注册清理处理器
	handler.AddCleanHandler(func() error {
		utils.Infof("BaseMappingHandler[%s]: cleaning up resources", config.MappingID)

		// 停止统计上报
		if handler.statsReportTicker != nil {
			handler.statsReportTicker.Stop()
		}

		// 最后一次上报流量统计
		handler.reportStats()

		// 关闭协议适配器
		return adapter.Close()
	})

	return handler
}

// Start 启动映射处理器
func (h *BaseMappingHandler) Start() error {
	// 1. 创建Transformer（公共）
	if err := h.createTransformer(); err != nil {
		return fmt.Errorf("failed to create transformer: %w", err)
	}

	// 2. 启动监听（委托给adapter）
	if err := h.adapter.StartListener(h.config); err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	utils.Infof("BaseMappingHandler: %s mapping started on port %d",
		h.adapter.GetProtocol(), h.config.LocalPort)

	// 3. 启动接受循环（公共）
	go h.acceptLoop()

	return nil
}

// acceptLoop 接受连接循环
func (h *BaseMappingHandler) acceptLoop() {
	for {
		select {
		case <-h.Ctx().Done():
			return
		default:
		}

		// 接受连接（委托给adapter）
		localConn, err := h.adapter.Accept()
		if err != nil {
			if h.Ctx().Err() != nil {
				return
			}
			// 记录错误但继续接受
			utils.Errorf("BaseMappingHandler[%s]: accept error: %v", h.config.MappingID, err)
			time.Sleep(100 * time.Millisecond) // 避免错误循环
			continue
		}

		// 处理连接（公共）
		go h.handleConnection(localConn)
	}
}

// handleConnection 处理单个连接
func (h *BaseMappingHandler) handleConnection(localConn io.ReadWriteCloser) {
	defer localConn.Close()

	utils.Debugf("BaseMappingHandler[%s]: new connection received", h.config.MappingID)

	// 1. 配额检查：连接数限制
	if err := h.checkConnectionQuota(); err != nil {
		utils.Warnf("BaseMappingHandler[%s]: quota check failed: %v", h.config.MappingID, err)
		return
	}

	// 增加活跃连接计数
	currentCount := h.activeConnCount.Add(1)
	defer h.activeConnCount.Add(-1)

	utils.Debugf("BaseMappingHandler[%s]: active connections: %d", h.config.MappingID, currentCount)

	// 2. 连接预处理（委托给adapter）
	if err := h.adapter.PrepareConnection(localConn); err != nil {
		utils.Errorf("BaseMappingHandler[%s]: prepare connection failed: %v", h.config.MappingID, err)
		return
	}

	// 3. 生成TunnelID
	tunnelID := h.generateTunnelID()

	// 4. 配额检查：流量限制
	if err := h.client.CheckMappingQuota(h.config.MappingID); err != nil {
		utils.Warnf("BaseMappingHandler[%s]: mapping quota exceeded: %v", h.config.MappingID, err)
		return
	}

	// 5. 建立隧道连接
	utils.Debugf("BaseMappingHandler[%s]: dialing tunnel %s", h.config.MappingID, tunnelID)
	tunnelConn, tunnelStream, err := h.client.DialTunnel(
		tunnelID,
		h.config.MappingID,
		h.config.SecretKey,
	)
	if err != nil {
		utils.Errorf("BaseMappingHandler[%s]: dial tunnel failed: %v", h.config.MappingID, err)
		return
	}
	defer tunnelConn.Close()

	utils.Debugf("BaseMappingHandler[%s]: tunnel %s established", h.config.MappingID, tunnelID)

	// 6. ✅ 使用StreamProcessor的Reader/Writer进行透传
	// StreamProcessor已经包含了压缩/加密层，不应该关闭它或绕过它
	tunnelReader := tunnelStream.GetReader()
	tunnelWriter := tunnelStream.GetWriter()
	
	// 7. 包装隧道连接成ReadWriteCloser
	tunnelRWC := utils.NewReadWriteCloser(tunnelReader, tunnelWriter, func() error {
	tunnelStream.Close()
		tunnelConn.Close()
		return nil
	})

	// 8. 双向转发（Transformer只处理限速，压缩/加密已在StreamProcessor中）
	utils.BidirectionalCopy(localConn, tunnelRWC, &utils.BidirectionalCopyOptions{
		Transformer: h.transformer,
		LogPrefix:   fmt.Sprintf("BaseMappingHandler[%s][%s]", h.config.MappingID, tunnelID),
	})

	// 9. 更新连接计数统计
	h.trafficStats.ConnectionCount.Add(1)
}

// checkConnectionQuota 检查连接数配额
func (h *BaseMappingHandler) checkConnectionQuota() error {
	// 优先使用mapping配置的连接数限制
	maxConn := h.config.MaxConnections
	if maxConn <= 0 {
		// 如果mapping未设置，使用用户配额的全局限制
		quota, err := h.client.GetUserQuota()
		if err != nil {
			// 如果获取配额失败，记录日志但不阻塞连接
			utils.Warnf("BaseMappingHandler[%s]: failed to get quota: %v", h.config.MappingID, err)
			return nil
		}
		maxConn = quota.MaxConnections
	}

	// 检查连接数限制
	if maxConn > 0 {
		if int(h.activeConnCount.Load()) >= maxConn {
			return fmt.Errorf("max connections reached: %d/%d", h.activeConnCount.Load(), maxConn)
		}
	}

	return nil
}

// wrapConnectionForControl 包装连接以进行速率限制和流量统计
func (h *BaseMappingHandler) wrapConnectionForControl(
	conn io.ReadWriteCloser,
	direction string,
) io.ReadWriteCloser {
	return &controlledConn{
		ReadWriteCloser: conn,
		rateLimiter:     h.rateLimiter,
		stats:           h.trafficStats,
		direction:       direction,
	}
}

// createTransformer 创建流转换器
// 注意：压缩和加密已移至StreamProcessor，Transform只处理限速
func (h *BaseMappingHandler) createTransformer() error {
	transformConfig := &transform.TransformConfig{
		BandwidthLimit: h.config.BandwidthLimit,
	}

	transformer, err := transform.NewTransformer(transformConfig)
	if err != nil {
		return err
	}

	h.transformer = transformer
	utils.Debugf("BaseMappingHandler[%s]: transformer created, bandwidth_limit=%d bytes/s",
		h.config.MappingID, h.config.BandwidthLimit)
	return nil
}

// generateTunnelID 生成隧道ID
func (h *BaseMappingHandler) generateTunnelID() string {
	return fmt.Sprintf("%s-tunnel-%d-%d",
		h.adapter.GetProtocol(),
		time.Now().UnixNano(),
		h.config.LocalPort,
	)
}

// reportStatsLoop 定期上报流量统计
func (h *BaseMappingHandler) reportStatsLoop() {
	for {
		select {
		case <-h.Ctx().Done():
			return
		case <-h.statsReportTicker.C:
			h.reportStats()
		}
	}
}

// reportStats 上报流量统计
func (h *BaseMappingHandler) reportStats() {
	bytesSent := h.trafficStats.BytesSent.Swap(0)
	bytesReceived := h.trafficStats.BytesReceived.Swap(0)

	if bytesSent > 0 || bytesReceived > 0 {
		if err := h.client.TrackTraffic(h.config.MappingID, bytesSent, bytesReceived); err != nil {
			utils.Warnf("BaseMappingHandler[%s]: failed to report stats: %v", h.config.MappingID, err)
			// 回滚计数（避免丢失）
			h.trafficStats.BytesSent.Add(bytesSent)
			h.trafficStats.BytesReceived.Add(bytesReceived)
		} else {
			utils.Debugf("BaseMappingHandler[%s]: reported stats - sent=%d, received=%d",
				h.config.MappingID, bytesSent, bytesReceived)
		}
	}
}

// Stop 停止映射处理器
func (h *BaseMappingHandler) Stop() {
	utils.Infof("BaseMappingHandler[%s]: stopping", h.config.MappingID)
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

// controlledConn 包装的连接（带速率限制和流量统计）
type controlledConn struct {
	io.ReadWriteCloser
	rateLimiter *rate.Limiter
	stats       *TrafficStats
	direction   string // "local" or "tunnel"
}

func (c *controlledConn) Read(p []byte) (n int, err error) {
	// 速率限制（如果启用）
	if c.rateLimiter != nil {
		if err := c.rateLimiter.WaitN(context.Background(), len(p)); err != nil {
			return 0, err
		}
	}

	// 读取数据
	n, err = c.ReadWriteCloser.Read(p)

	// 流量统计
	if n > 0 {
		if c.direction == "tunnel" {
			c.stats.BytesReceived.Add(int64(n))
		} else {
			c.stats.BytesSent.Add(int64(n))
		}
	}

	return n, err
}

func (c *controlledConn) Write(p []byte) (n int, err error) {
	// 速率限制（如果启用）
	if c.rateLimiter != nil {
		if err := c.rateLimiter.WaitN(context.Background(), len(p)); err != nil {
			return 0, err
		}
	}

	// 写入数据
	n, err = c.ReadWriteCloser.Write(p)

	// 流量统计
	if n > 0 {
		if c.direction == "tunnel" {
			c.stats.BytesSent.Add(int64(n))
		} else {
			c.stats.BytesReceived.Add(int64(n))
		}
	}

	return n, err
}

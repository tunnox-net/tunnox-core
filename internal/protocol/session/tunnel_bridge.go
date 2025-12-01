package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

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

	tunnelID     string
	mappingID    string   // 映射ID（用于流量统计）
	sourceConn   net.Conn // 源端客户端的隧道连接
	sourceStream stream.PackageStreamer
	sourceConnMu sync.RWMutex // 保护 sourceConn 的读写
	targetConn   net.Conn     // 目标端客户端的隧道连接（等待建立）
	targetStream stream.PackageStreamer
	ready        chan struct{} // 用于通知目标端连接已建立

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
	TunnelID       string
	MappingID      string
	SourceConn     net.Conn
	SourceStream   stream.PackageStreamer
	BandwidthLimit int64           // 字节/秒，0表示不限制
	CloudControl   CloudControlAPI // 用于上报流量统计（可选）
}

// NewTunnelBridge 创建隧道桥接器
func NewTunnelBridge(parentCtx context.Context, config *TunnelBridgeConfig) *TunnelBridge {
	bridge := &TunnelBridge{
		ManagerBase:  dispose.NewManager(fmt.Sprintf("TunnelBridge-%s", config.TunnelID), parentCtx),
		tunnelID:     config.TunnelID,
		mappingID:    config.MappingID,
		sourceConn:   config.SourceConn,
		sourceStream: config.SourceStream,
		cloudControl: config.CloudControl,
		ready:        make(chan struct{}),
	}

	// 配置带宽限制
	if config.BandwidthLimit > 0 {
		// Token Bucket: limit = 每秒字节数, burst = 2倍limit（允许短时突发）
		bridge.rateLimiter = rate.NewLimiter(rate.Limit(config.BandwidthLimit), int(config.BandwidthLimit*2))
		utils.Infof("TunnelBridge[%s]: bandwidth limit set to %d bytes/sec", config.TunnelID, config.BandwidthLimit)
	}

	// 注册清理处理器
	bridge.AddCleanHandler(func() error {
		utils.Infof("TunnelBridge[%s]: cleaning up resources", config.TunnelID)

		// 上报最终流量统计
		bridge.reportTrafficStats()

		var errs []error

		// 关闭 StreamProcessor
		if bridge.sourceStream != nil {
			bridge.sourceStream.Close()
		}
		if bridge.targetStream != nil {
			bridge.targetStream.Close()
		}

		// 关闭底层连接
		if bridge.sourceConn != nil {
			if err := bridge.sourceConn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("source conn close error: %w", err))
			}
		}
		if bridge.targetConn != nil {
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

// SetTargetConnection 设置目标端连接
func (b *TunnelBridge) SetTargetConnection(targetConn net.Conn, targetStream stream.PackageStreamer) {
	b.targetConn = targetConn
	b.targetStream = targetStream
	close(b.ready)
}

// SetSourceConnection 设置源端连接（用于更新连接）
func (b *TunnelBridge) SetSourceConnection(sourceConn net.Conn, sourceStream stream.PackageStreamer) {
	b.sourceConnMu.Lock()
	oldConn := b.sourceConn
	b.sourceConn = sourceConn
	b.sourceConnMu.Unlock()
	if sourceStream != nil {
		b.sourceStream = sourceStream
	}
	utils.Infof("TunnelBridge[%s]: updated sourceConn, mappingID=%s, oldConn=%v, newConn=%v",
		b.tunnelID, b.mappingID, oldConn, sourceConn)
}

// getSourceConn 获取源端连接（线程安全）
func (b *TunnelBridge) getSourceConn() net.Conn {
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceConn
}

// Start 启动桥接
func (b *TunnelBridge) Start() error {
	// 等待目标端连接建立（超时30秒）
	select {
	case <-b.ready:
		utils.Infof("TunnelBridge[%s]: target connection established, starting bridge", b.tunnelID)
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for target connection")
	case <-b.Ctx().Done():
		return fmt.Errorf("bridge cancelled before target connection")
	}

	if b.sourceConn == nil || b.targetConn == nil {
		return fmt.Errorf("source or target connection not set")
	}

	// ✅ 服务端是透明桥接，直接使用原始net.Conn转发（不解压不解密）
	// 压缩/加密由客户端两端处理，服务端只负责纯转发
	utils.Infof("TunnelBridge[%s]: bridge started, transparent forwarding (no compression/encryption on server)", b.tunnelID)

	// 启动双向数据转发（带流量统计和限速）
	// 源端 -> 目标端（带限速和统计）
	// ✅ 使用动态获取 sourceConn 的方式，支持连接更新
	go func() {
		for {
			sourceConn := b.getSourceConn()
			if sourceConn == nil {
				utils.Warnf("TunnelBridge[%s]: sourceConn is nil, waiting...", b.tunnelID)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			utils.Infof("TunnelBridge[%s]: starting source->target copy, sourceConn=%v", b.tunnelID, sourceConn)
			written := b.copyWithControl(b.targetConn, sourceConn, "source->target", &b.bytesSent)
			utils.Infof("TunnelBridge[%s]: source->target copy finished, %d bytes", b.tunnelID, written)
			// 如果读取完成（EOF 或错误），检查是否有新的连接
			// 如果有新连接，继续读取；否则退出
			newSourceConn := b.getSourceConn()
			if newSourceConn == nil {
				// 连接被清空，退出
				utils.Infof("TunnelBridge[%s]: sourceConn is nil, exiting", b.tunnelID)
				break
			}
			if newSourceConn == sourceConn {
				// 连接没有更新，正常退出（EOF 或错误）
				utils.Infof("TunnelBridge[%s]: sourceConn unchanged (conn=%v), exiting", b.tunnelID, sourceConn)
				break
			}
			// 连接已更新，继续使用新连接读取
			utils.Infof("TunnelBridge[%s]: sourceConn updated (old=%v, new=%v), continuing with new connection",
				b.tunnelID, sourceConn, newSourceConn)
		}
	}()

	// 目标端 -> 源端（带限速和统计）
	// ✅ 使用动态获取 sourceConn 的方式，支持连接更新
	// 注意：target->source 方向是从 targetConn 读取，写入到 sourceConn
	// 所以需要在每次写入时获取最新的 sourceConn
	go func() {
		// 创建一个包装器，每次写入时都获取最新的 sourceConn
		dynamicWriter := &dynamicSourceWriter{bridge: b}
		written := b.copyWithControl(dynamicWriter, b.targetConn, "target->source", &b.bytesReceived)
		utils.Infof("TunnelBridge[%s]: target->source finished, %d bytes", b.tunnelID, written)
	}()

	// 启动定期流量统计上报（每30秒）
	if b.cloudControl != nil && b.mappingID != "" {
		go b.periodicTrafficReport()
	}

	return nil
}

// periodicTrafficReport 定期上报流量统计
func (b *TunnelBridge) periodicTrafficReport() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.reportTrafficStats()
		case <-b.Ctx().Done():
			// 最终上报
			b.reportTrafficStats()
			return
		}
	}
}

// copyWithControl 带流量统计和限速的数据拷贝
func (b *TunnelBridge) copyWithControl(dst io.Writer, src io.Reader, direction string, counter *atomic.Int64) int64 {
	buf := make([]byte, 32*1024) // 32KB buffer
	var total int64

	for {
		// 检查是否已取消
		select {
		case <-b.Ctx().Done():
			utils.Debugf("TunnelBridge[%s]: %s cancelled", b.tunnelID, direction)
			return total
		default:
		}

		// 从源端读取（带超时检测）
		// ✅ 提取连接的 connID 用于调试
		srcConnID := "unknown"
		if srcConn, ok := src.(interface{ GetConnectionID() string }); ok {
			srcConnID = srcConn.GetConnectionID()
		} else if srcConn, ok := src.(interface{ GetClientID() int64 }); ok {
			srcConnID = fmt.Sprintf("client_%d", srcConn.GetClientID())
		}
		utils.Infof("TunnelBridge[%s]: %s calling src.Read(buf), src type=%T, srcConnID=%s, buf size=%d", b.tunnelID, direction, src, srcConnID, len(buf))
		nr, err := src.Read(buf)
		firstByte := byte(0)
		if nr > 0 {
			firstByte = buf[0]
		}
		utils.Infof("TunnelBridge[%s]: %s src.Read returned, n=%d, err=%v, firstByte=0x%02x, srcConnID=%s", b.tunnelID, direction, nr, err, firstByte, srcConnID)
		if nr > 0 {
			firstByte := byte(0)
			if len(buf) > 0 {
				firstByte = buf[0]
			}
			utils.Infof("TunnelBridge[%s]: %s read %d bytes, firstByte=0x%02x", b.tunnelID, direction, nr, firstByte)
			// 应用限速（如果启用）
			if b.rateLimiter != nil {
				// 使用 bridge 的 context 进行限速等待
				if err := b.rateLimiter.WaitN(b.Ctx(), nr); err != nil {
					utils.Errorf("TunnelBridge[%s]: %s rate limit error: %v", b.tunnelID, direction, err)
					break
				}
			}

			// 写入目标端
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				total += int64(nw)
				counter.Add(int64(nw)) // 更新流量统计
				utils.Infof("TunnelBridge[%s]: %s wrote %d bytes (total: %d)", b.tunnelID, direction, nw, total)
			}
			if ew != nil {
				if ew != io.EOF {
					utils.Debugf("TunnelBridge[%s]: %s write error: %v", b.tunnelID, direction, ew)
				}
				break
			}
			if nr != nw {
				utils.Errorf("TunnelBridge[%s]: %s short write", b.tunnelID, direction)
				break
			}
		}
		if err != nil {
			// ✅ UDP 连接的超时错误是临时错误，不应该导致连接关闭
			if netErr, ok := err.(interface {
				Timeout() bool
				Temporary() bool
			}); ok && netErr.Timeout() && netErr.Temporary() {
				// UDP 超时错误，继续等待
				utils.Debugf("TunnelBridge[%s]: %s UDP timeout, continuing...", b.tunnelID, direction)
				continue
			}
			if err != io.EOF {
				utils.Infof("TunnelBridge[%s]: %s read error: %v (total bytes: %d)", b.tunnelID, direction, err, total)
			} else {
				utils.Infof("TunnelBridge[%s]: %s read EOF (total bytes: %d)", b.tunnelID, direction, total)
			}
			break
		}
	}

	return total
}

// reportTrafficStats 上报流量统计到CloudControl
func (b *TunnelBridge) reportTrafficStats() {
	if b.cloudControl == nil || b.mappingID == "" {
		return
	}

	// 获取当前累计值
	currentSent := b.bytesSent.Load()
	currentReceived := b.bytesReceived.Load()

	// 获取上次上报的值
	lastSent := b.lastReportedSent.Load()
	lastReceived := b.lastReportedReceived.Load()

	// 计算增量
	deltaSent := currentSent - lastSent
	deltaReceived := currentReceived - lastReceived

	// 如果没有增量，不上报
	if deltaSent == 0 && deltaReceived == 0 {
		return
	}

	// 获取当前映射的统计数据
	mapping, err := b.cloudControl.GetPortMapping(b.mappingID)
	if err != nil {
		utils.Errorf("TunnelBridge[%s]: failed to get mapping for traffic stats: %v", b.tunnelID, err)
		return
	}

	// 累加增量到映射统计
	trafficStats := mapping.TrafficStats
	trafficStats.BytesSent += deltaSent
	trafficStats.BytesReceived += deltaReceived
	trafficStats.LastUpdated = time.Now()

	// 更新映射统计
	if err := b.cloudControl.UpdatePortMappingStats(b.mappingID, &trafficStats); err != nil {
		utils.Errorf("TunnelBridge[%s]: failed to update traffic stats: %v", b.tunnelID, err)
		return
	}

	// 更新上次上报的值
	b.lastReportedSent.Store(currentSent)
	b.lastReportedReceived.Store(currentReceived)

	utils.Infof("TunnelBridge[%s]: traffic stats updated - mapping=%s, delta_sent=%d, delta_received=%d, total_sent=%d, total_received=%d",
		b.tunnelID, b.mappingID, deltaSent, deltaReceived, trafficStats.BytesSent, trafficStats.BytesReceived)
}

// Close 关闭桥接
func (b *TunnelBridge) Close() error {
	b.ManagerBase.Close()
	return nil
}

// dynamicSourceWriter 动态获取 sourceConn 的 Writer 包装器
// 用于在 target->source 方向时，每次写入都使用最新的 sourceConn
type dynamicSourceWriter struct {
	bridge *TunnelBridge
}

func (w *dynamicSourceWriter) Write(p []byte) (n int, err error) {
	sourceConn := w.bridge.getSourceConn()
	if sourceConn == nil {
		return 0, fmt.Errorf("sourceConn is nil")
	}
	return sourceConn.Write(p)
}

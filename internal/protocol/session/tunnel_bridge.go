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
	targetConn   net.Conn // 目标端客户端的隧道连接（等待建立）
	targetStream stream.PackageStreamer
	ready        chan struct{} // 用于通知目标端连接已建立

	// 商业特性
	rateLimiter   *rate.Limiter   // 带宽限制器
	bytesSent     atomic.Int64    // 源端→目标端字节数
	bytesReceived atomic.Int64    // 目标端→源端字节数
	cloudControl  CloudControlAPI // 用于上报流量统计
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

// Start 启动桥接（阻塞直到连接关闭）
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

	// ✅ 服务端是透明桥接，直接使用原始net.Conn转发（不解压不解密）
	// 压缩/加密由客户端两端处理，服务端只负责纯转发
	utils.Infof("TunnelBridge[%s]: bridge started, transparent forwarding (no compression/encryption on server)", b.tunnelID)

	// 启动双向数据转发（带流量统计和限速）
	var wg sync.WaitGroup
	wg.Add(2)

	// 源端 -> 目标端（带限速和统计）
	go func() {
		defer wg.Done()
		written := b.copyWithControl(b.targetConn, b.sourceConn, "source->target", &b.bytesSent)
		utils.Infof("TunnelBridge[%s]: source->target finished, %d bytes", b.tunnelID, written)
	}()

	// 目标端 -> 源端（带限速和统计）
	go func() {
		defer wg.Done()
		written := b.copyWithControl(b.sourceConn, b.targetConn, "target->source", &b.bytesReceived)
		utils.Infof("TunnelBridge[%s]: target->source finished, %d bytes", b.tunnelID, written)
	}()

	wg.Wait()

	utils.Infof("TunnelBridge[%s]: bridge finished (sent: %d, received: %d)",
		b.tunnelID, b.bytesSent.Load(), b.bytesReceived.Load())
	return nil
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

		// 从源端读取
		nr, err := src.Read(buf)
		if nr > 0 {
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
			if err != io.EOF {
				utils.Debugf("TunnelBridge[%s]: %s read error: %v", b.tunnelID, direction, err)
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

	sent := b.bytesSent.Load()
	received := b.bytesReceived.Load()

	if sent == 0 && received == 0 {
		return // 无流量，不上报
	}

	// 获取当前映射的统计数据
	mapping, err := b.cloudControl.GetPortMapping(b.mappingID)
	if err != nil {
		utils.Errorf("TunnelBridge[%s]: failed to get mapping for traffic stats: %v", b.tunnelID, err)
		return
	}

	// 累加流量统计
	trafficStats := mapping.TrafficStats
	trafficStats.BytesSent += sent
	trafficStats.BytesReceived += received
	trafficStats.LastUpdated = time.Now()

	// 更新映射统计
	if err := b.cloudControl.UpdatePortMappingStats(b.mappingID, &trafficStats); err != nil {
		utils.Errorf("TunnelBridge[%s]: failed to update traffic stats: %v", b.tunnelID, err)
		return
	}

	utils.Infof("TunnelBridge[%s]: traffic stats updated - mapping=%s, sent=%d, received=%d, total_sent=%d, total_received=%d",
		b.tunnelID, b.mappingID, sent, received, trafficStats.BytesSent, trafficStats.BytesReceived)
}

// Close 关闭桥接
func (b *TunnelBridge) Close() error {
	b.ManagerBase.Close()
	return nil
}

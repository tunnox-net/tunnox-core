package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

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
	tunnelID     string
	mappingID    string   // 映射ID（用于流量统计）
	sourceConn   net.Conn // 源端客户端的隧道连接
	sourceStream stream.PackageStreamer
	targetConn   net.Conn // 目标端客户端的隧道连接（等待建立）
	targetStream stream.PackageStreamer
	ready        chan struct{} // 用于通知目标端连接已建立
	closed       bool
	closeMutex   sync.Mutex

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
func NewTunnelBridge(config *TunnelBridgeConfig) *TunnelBridge {
	bridge := &TunnelBridge{
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

	// 上报最终流量统计
	b.reportTrafficStats()

	// ✅ 在两个方向都完成后再关闭连接
	b.Close()

	utils.Infof("TunnelBridge[%s]: bridge closed (sent: %d, received: %d)",
		b.tunnelID, b.bytesSent.Load(), b.bytesReceived.Load())
	return nil
}

// copyWithControl 带流量统计和限速的数据拷贝
func (b *TunnelBridge) copyWithControl(dst io.Writer, src io.Reader, direction string, counter *atomic.Int64) int64 {
	buf := make([]byte, 32*1024) // 32KB buffer
	var total int64

	for {
		// 从源端读取
		nr, err := src.Read(buf)
		if nr > 0 {
			// 应用限速（如果启用）
			if b.rateLimiter != nil {
				// 等待令牌桶允许
				if err := b.rateLimiter.WaitN(context.Background(), nr); err != nil {
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

	// CloudControlAPI.ReportTraffic 尚未实现，目前仅记录日志
	utils.Infof("TunnelBridge[%s]: traffic stats - mapping=%s, sent=%d, received=%d",
		b.tunnelID, b.mappingID, sent, received)
}

// Close 关闭桥接
func (b *TunnelBridge) Close() {
	b.closeMutex.Lock()
	defer b.closeMutex.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	// ✅ 关闭StreamProcessor会自动关闭底层连接
	if b.sourceStream != nil {
		b.sourceStream.Close()
	}
	if b.targetStream != nil {
		b.targetStream.Close()
	}

	// 也关闭底层连接（双保险）
	if b.sourceConn != nil {
		b.sourceConn.Close()
	}
	if b.targetConn != nil {
		b.targetConn.Close()
	}
}

package session

import (
	"fmt"
	"time"

	"tunnox-core/internal/utils"
)

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

	// 检查数据转发器是否可用（通过接口抽象，不依赖具体协议）
	if b.sourceForwarder == nil {
		// 尝试重新创建（可能是在 SetTargetConnection 之后才设置 source）
		utils.Infof("TunnelBridge[%s]: recreating sourceForwarder, sourceConn=%v, sourceStream=%v", b.tunnelID, b.sourceConn != nil, b.sourceStream != nil)
		b.sourceForwarder = createDataForwarder(b.sourceConn, b.sourceStream)
	}
	if b.targetForwarder == nil {
		// 尝试重新创建（可能是在 SetSourceConnection 之后才设置 target）
		utils.Infof("TunnelBridge[%s]: recreating targetForwarder, targetConn=%v, targetStream=%v", b.tunnelID, b.targetConn != nil, b.targetStream != nil)
		b.targetForwarder = createDataForwarder(b.targetConn, b.targetStream)
	}

	// 如果源端或目标端没有数据转发器，说明该协议不支持桥接（如 HTTP 长轮询）
	// 数据已经通过协议本身传输，只需要管理连接生命周期
	if b.sourceForwarder == nil || b.targetForwarder == nil {
		utils.Infof("TunnelBridge[%s]: connection does not support data forwarding (sourceForwarder=%v, targetForwarder=%v), bridge only manages connection lifecycle",
			b.tunnelID, b.sourceForwarder != nil, b.targetForwarder != nil)
		if b.cloudControl != nil && b.mappingID != "" {
			go b.periodicTrafficReport()
		}
		return nil
	}

	// ✅ 服务端是透明桥接，直接使用原始net.Conn转发（不解压不解密）
	// 压缩/加密由客户端两端处理，服务端只负责纯转发
	utils.Infof("TunnelBridge[%s]: bridge started, transparent forwarding (no compression/encryption on server)", b.tunnelID)

	// 启动双向数据转发（带流量统计和限速）
	// 源端 -> 目标端（带限速和统计）
	// ✅ 使用接口抽象，支持不同协议
	go func() {
		for {
			b.sourceConnMu.RLock()
			sourceForwarder := b.sourceForwarder
			b.sourceConnMu.RUnlock()

			if sourceForwarder == nil {
				utils.Warnf("TunnelBridge[%s]: sourceForwarder is nil, waiting...", b.tunnelID)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			utils.Infof("TunnelBridge[%s]: starting source->target copy", b.tunnelID)
			written := b.copyWithControl(b.targetForwarder, sourceForwarder, "source->target", &b.bytesSent)
			utils.Infof("TunnelBridge[%s]: source->target copy finished, %d bytes", b.tunnelID, written)

			// 检查连接是否更新
			b.sourceConnMu.RLock()
			newSourceForwarder := b.sourceForwarder
			b.sourceConnMu.RUnlock()

			if newSourceForwarder == nil {
				utils.Infof("TunnelBridge[%s]: sourceForwarder is nil, exiting", b.tunnelID)
				break
			}
			if newSourceForwarder == sourceForwarder {
				utils.Infof("TunnelBridge[%s]: sourceForwarder unchanged, exiting", b.tunnelID)
				break
			}
			utils.Infof("TunnelBridge[%s]: sourceForwarder updated, continuing with new connection", b.tunnelID)
		}
	}()

	// 目标端 -> 源端（带限速和统计）
	// ✅ 使用接口抽象，支持不同协议
	go func() {
		// 创建一个包装器，每次写入时都获取最新的 sourceForwarder
		dynamicWriter := &dynamicSourceWriter{bridge: b}
		written := b.copyWithControl(dynamicWriter, b.targetForwarder, "target->source", &b.bytesReceived)
		utils.Infof("TunnelBridge[%s]: target->source finished, %d bytes", b.tunnelID, written)
	}()

	// 启动定期流量统计上报（每30秒）
	if b.cloudControl != nil && b.mappingID != "" {
		go b.periodicTrafficReport()
	}

	return nil
}

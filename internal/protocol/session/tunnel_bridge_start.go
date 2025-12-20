package session

import (
	"fmt"
	"time"
)

// Start 启动桥接（高性能版本）
func (b *TunnelBridge) Start() error {
	// 等待目标端连接建立（超时30秒）
	select {
	case <-b.ready:
		// 目标连接已建立
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for target connection")
	case <-b.Ctx().Done():
		return fmt.Errorf("bridge cancelled before target connection")
	}

	// 检查数据转发器是否可用
	if b.sourceForwarder == nil {
		b.sourceForwarder = createDataForwarder(b.sourceConn, b.sourceStream)
	}
	if b.targetForwarder == nil {
		b.targetForwarder = createDataForwarder(b.targetConn, b.targetStream)
	}

	// 如果源端或目标端没有数据转发器，只管理连接生命周期
	if b.sourceForwarder == nil || b.targetForwarder == nil {
		if b.cloudControl != nil && b.mappingID != "" {
			go b.periodicTrafficReport()
		}
		return nil
	}

	// 启动双向数据转发
	// 源端 -> 目标端
	go func() {
		for {
			b.sourceConnMu.RLock()
			sourceForwarder := b.sourceForwarder
			b.sourceConnMu.RUnlock()

			if sourceForwarder == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			b.copyWithControl(b.targetForwarder, sourceForwarder, "source->target", &b.bytesSent)

			// 检查连接是否更新
			b.sourceConnMu.RLock()
			newSourceForwarder := b.sourceForwarder
			b.sourceConnMu.RUnlock()

			if newSourceForwarder == nil || newSourceForwarder == sourceForwarder {
				break
			}
		}
	}()

	// 目标端 -> 源端
	go func() {
		dynamicWriter := &dynamicSourceWriter{bridge: b}
		b.copyWithControl(dynamicWriter, b.targetForwarder, "target->source", &b.bytesReceived)
	}()

	// 启动定期流量统计上报
	if b.cloudControl != nil && b.mappingID != "" {
		go b.periodicTrafficReport()
	}

	return nil
}

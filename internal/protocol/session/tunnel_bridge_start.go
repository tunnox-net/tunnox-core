package session

import (
	"fmt"
	"sync"
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

	// 跨节点场景：数据转发由 CrossNodeListener 负责，这里只管理生命周期
	if b.GetCrossNodeConnection() != nil {
		if b.cloudControl != nil && b.mappingID != "" {
			go b.periodicTrafficReport()
		}
		// 等待跨节点转发完成（由 CrossNodeListener.runBridgeForward 处理）
		<-b.Ctx().Done()
		return nil
	}

	// 检查数据转发器是否可用
	if b.sourceForwarder == nil {
		b.sourceForwarder = createDataForwarder(b.sourceConn, b.sourceStream)
	}
	if b.targetForwarder == nil {
		b.targetForwarder = createDataForwarder(b.targetConn, b.targetStream)
	}

	// 如果源端或目标端没有数据转发器，只管理连接生命周期
	// 🔧 修复：跨节点场景下，forwarder 可能为 nil（数据转发由 CrossNodeListener 负责）
	// 此时需要等待 context 完成，而不是直接返回
	if b.sourceForwarder == nil || b.targetForwarder == nil {
		if b.cloudControl != nil && b.mappingID != "" {
			go b.periodicTrafficReport()
		}
		// 等待 bridge 生命周期结束（由 CrossNodeListener 或其他组件触发 Close）
		<-b.Ctx().Done()
		return nil
	}

	// 🔧 修复：任一方向的数据传输结束后，关闭整个 bridge
	// 这样可以确保：
	// 1. listenClient 关闭连接后，Server 立即关闭 targetClient 方向的连接
	// 2. targetClient 检测到连接关闭，立即释放到后端服务（如 PostgreSQL）的连接
	var closeOnce sync.Once
	closeBridge := func() {
		closeOnce.Do(func() {
			b.Close()
		})
	}

	// 启动双向数据转发
	// 源端 -> 目标端
	go func() {
		defer closeBridge() // 🔧 数据传输结束后关闭 bridge

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
		defer closeBridge() // 🔧 数据传输结束后关闭 bridge

		dynamicWriter := &dynamicSourceWriter{bridge: b}
		b.copyWithControl(dynamicWriter, b.targetForwarder, "target->source", &b.bytesReceived)
	}()

	// 启动定期流量统计上报
	if b.cloudControl != nil && b.mappingID != "" {
		go b.periodicTrafficReport()
	}

	return nil
}

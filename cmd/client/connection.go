package main

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/client"
	corelog "tunnox-core/internal/core/log"
)

// connectWithRetry 带重试的连接
func connectWithRetry(tunnoxClient *client.TunnoxClient, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			fmt.Printf("🔄 Retry %d/%d...\n", i, maxRetries)
			time.Sleep(time.Duration(i) * 2 * time.Second) // 指数退避
		}

		if err := tunnoxClient.Connect(); err != nil {
			if i == maxRetries-1 {
				return err
			}
			fmt.Printf("⚠️  Connection failed: %v\n", err)
			continue
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded")
}

// monitorConnectionAndReconnect 监控连接状态并自动重连
// 注意：此函数仅作为备用重连机制，主要重连由 readLoop 退出时触发
// 如果 readLoop 的重连机制正常工作，此函数通常不会触发
func monitorConnectionAndReconnect(ctx context.Context, tunnoxClient *client.TunnoxClient) {
	ticker := time.NewTicker(30 * time.Second) // ✅ 增加检查间隔，避免与 readLoop 重连冲突
	defer ticker.Stop()

	consecutiveFailures := 0
	maxFailures := 3

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 检查连接状态
			// ✅ 仅在连接断开且持续一段时间后才触发重连（给 readLoop 的重连机制时间）
			if !tunnoxClient.IsConnected() {
				consecutiveFailures++
				corelog.Warnf("Connection lost (failure %d/%d), attempting to reconnect via monitor...",
					consecutiveFailures, maxFailures)

				// ✅ 使用 Reconnect() 方法，它内部已经有防重复重连机制
				if err := tunnoxClient.Reconnect(); err != nil {
					corelog.Errorf("Reconnection failed: %v", err)

					if consecutiveFailures >= maxFailures {
						corelog.Errorf("Max reconnection attempts reached, giving up")
						return
					}
				} else {
					corelog.Infof("Reconnected successfully via monitor")
					consecutiveFailures = 0
				}
			} else {
				// 连接正常，重置失败计数
				if consecutiveFailures > 0 {
					consecutiveFailures = 0
				}
			}
		}
	}
}

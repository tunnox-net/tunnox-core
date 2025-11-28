package client

import (
	"time"
	"tunnox-core/internal/utils"
)

// ReconnectConfig 重连配置
type ReconnectConfig struct {
	Enabled      bool          // 是否启用重连
	InitialDelay time.Duration // 初始延迟（1秒）
	MaxDelay     time.Duration // 最大延迟（60秒）
	MaxAttempts  int           // 最大尝试次数（0=无限）
	Backoff      float64       // 退避因子（2.0=指数退避）
}

// DefaultReconnectConfig 默认重连配置
var DefaultReconnectConfig = ReconnectConfig{
	Enabled:      true,
	InitialDelay: 200 * time.Millisecond, // ✅ 优化：缩短初始延迟到 200ms，加快重连速度
	MaxDelay:     60 * time.Second,
	MaxAttempts:  0, // 无限重试
	Backoff:      2.0,
}

// shouldReconnect 判断是否应该重连
func (c *TunnoxClient) shouldReconnect() bool {
	// 被踢下线不重连
	if c.kicked {
		utils.Infof("Client: not reconnecting (kicked by server)")
		return false
	}

	// 认证失败不重连
	if c.authFailed {
		utils.Infof("Client: not reconnecting (authentication failed)")
		return false
	}

	// 主动关闭不重连
	select {
	case <-c.Ctx().Done():
		utils.Infof("Client: not reconnecting (context cancelled)")
		return false
	default:
	}

	return true
}

// reconnect 重连逻辑
func (c *TunnoxClient) reconnect() {
	// ✅ 防止重复重连：使用原子操作检查是否已有重连在进行
	if !c.reconnecting.CompareAndSwap(false, true) {
		utils.Debugf("Client: reconnect already in progress, skipping")
		return
	}
	defer c.reconnecting.Store(false)

	// 获取重连配置
	reconnectConfig := c.getReconnectConfig()

	if !reconnectConfig.Enabled {
		utils.Infof("Client: reconnect disabled")
		return
	}

	delay := reconnectConfig.InitialDelay
	attempts := 0

	for {
		// 检查是否应该重连
		if !c.shouldReconnect() {
			return
		}

		// 检查最大尝试次数
		if reconnectConfig.MaxAttempts > 0 && attempts >= reconnectConfig.MaxAttempts {
			utils.Errorf("Client: max reconnect attempts (%d) reached, giving up", reconnectConfig.MaxAttempts)
			return
		}

		utils.Infof("Client: reconnecting in %v (attempt %d)...", delay, attempts+1)

		// ✅ 在等待期间检查 context，如果被取消则退出
		select {
		case <-c.Ctx().Done():
			utils.Infof("Client: reconnect cancelled (context done)")
			return
		case <-time.After(delay):
			// 继续重连尝试
		}

		// ✅ 在尝试连接前再次检查是否应该重连（防止在等待期间 context 被取消）
		if !c.shouldReconnect() {
			return
		}

		// 尝试重连
		if err := c.Connect(); err != nil {
			utils.Errorf("Client: reconnect failed: %v", err)

			// 增加延迟（指数退避）
			delay = time.Duration(float64(delay) * reconnectConfig.Backoff)
			if delay > reconnectConfig.MaxDelay {
				delay = reconnectConfig.MaxDelay
			}
			attempts++
			continue
		}

		utils.Infof("Client: reconnected successfully")

		// ✅ 重连成功后，恢复映射配置
		// 通过 ConfigGet 命令获取服务器端的映射配置
		if c.config.ClientID > 0 {
			go c.requestMappingConfig()
		}

		return
	}
}

// getReconnectConfig 获取重连配置
func (c *TunnoxClient) getReconnectConfig() ReconnectConfig {
	// 使用默认配置
	// 注意：如果需要从配置文件读取，需要在 ClientConfig 中添加 Reconnect 字段
	return DefaultReconnectConfig
}

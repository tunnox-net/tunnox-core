package client

import (
	"strings"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// connectWithAutoDetection 使用自动连接检测连接到服务器
func (c *TunnoxClient) connectWithAutoDetection() error {
	connector := NewAutoConnector(c.Ctx(), c)
	defer connector.Close()

	attempt, err := connector.ConnectWithAutoDetection(c.Ctx())
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "auto connection failed")
	}

	// 更新配置（更新内存中的配置）
	// 标记使用了自动连接，后续保存凭据时会同时保存服务器配置
	c.config.Server.Protocol = attempt.Endpoint.Protocol
	c.config.Server.Address = attempt.Endpoint.Address
	c.usedAutoConnection = true // 标记使用了自动连接检测

	corelog.Infof("Client: auto-detected server endpoint - %s://%s", attempt.Endpoint.Protocol, attempt.Endpoint.Address)

	// 使用已建立的连接和 Stream（握手已在 ConnectWithAutoDetection 中完成）
	c.mu.Lock()
	c.controlConn = attempt.Conn
	c.controlStream = attempt.Stream
	c.mu.Unlock()

	// 记录连接信息
	localAddr := "unknown"
	remoteAddr := "unknown"
	if attempt.Conn.LocalAddr() != nil {
		localAddr = attempt.Conn.LocalAddr().String()
	}
	if attempt.Conn.RemoteAddr() != nil {
		remoteAddr = attempt.Conn.RemoteAddr().String()
	}
	corelog.Infof("Client: %s connection established and handshake completed - Local=%s, Remote=%s",
		strings.ToUpper(attempt.Endpoint.Protocol), localAddr, remoteAddr)

	// 启动读取循环（接收服务器命令）
	if !c.readLoopRunning.CompareAndSwap(false, true) {
		corelog.Warnf("Client: readLoop already running, skipping")
	} else {
		go func() {
			defer c.readLoopRunning.Store(false)
			c.readLoop()
		}()
	}

	// 启动心跳循环
	if !c.heartbeatLoopRunning.CompareAndSwap(false, true) {
		corelog.Debugf("Client: heartbeatLoop already running, skipping")
	} else {
		go func() {
			defer c.heartbeatLoopRunning.Store(false)
			c.heartbeatLoop()
		}()
	}

	corelog.Infof("Client: control connection established successfully")
	return nil
}

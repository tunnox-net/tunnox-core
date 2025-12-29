package client

import (
	"fmt"
	"strings"

	"tunnox-core/internal/client/transport"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"
)

// connectWithAutoDetection 使用自动连接检测连接到服务器
func (c *TunnoxClient) connectWithAutoDetection() error {
	connector := NewAutoConnector(c.Ctx(), c)
	defer connector.Close()

	attempt, err := connector.ConnectWithAutoDetection(c.Ctx())
	if err != nil {
		return fmt.Errorf("auto connection failed: %w", err)
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

// connectWithEndpoint 使用指定的协议和地址建立控制连接
func (c *TunnoxClient) connectWithEndpoint(protocol, address string) error {
	corelog.Infof("Client: connecting to server %s://%s", protocol, address)

	protocol = strings.ToLower(protocol)

	// 检查协议是否可用
	if !transport.IsProtocolAvailable(protocol) {
		availableProtocols := transport.GetAvailableProtocolNames()
		return fmt.Errorf("protocol %q is not available (compiled protocols: %v)", protocol, availableProtocols)
	}

	// 使用统一的协议注册表拨号
	conn, err := transport.Dial(c.Ctx(), protocol, address)
	if err != nil {
		return fmt.Errorf("failed to dial server (%s): %w", protocol, err)
	}

	// TCP 特殊处理：设置 KeepAlive
	if protocol == "tcp" {
		SetKeepAliveIfSupported(conn, true)
	}

	c.config.Server.Protocol = strings.ToLower(protocol)

	// 使用锁保护连接状态
	c.mu.Lock()
	c.controlConn = conn
	streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
	c.controlStream = streamFactory.CreateStreamProcessor(conn, conn)
	c.mu.Unlock()

	// 记录连接信息
	localAddr := "unknown"
	remoteAddr := "unknown"
	if conn.LocalAddr() != nil {
		localAddr = conn.LocalAddr().String()
	}
	if conn.RemoteAddr() != nil {
		remoteAddr = conn.RemoteAddr().String()
	}
	corelog.Infof("Client: %s connection established - Local=%s, Remote=%s",
		strings.ToUpper(protocol), localAddr, remoteAddr)

	// 发送握手请求
	if err := c.sendHandshake(); err != nil {
		c.mu.Lock()
		if c.controlStream != nil {
			c.controlStream.Close()
			c.controlStream = nil
		}
		if c.controlConn != nil {
			c.controlConn.Close()
			c.controlConn = nil
		}
		c.mu.Unlock()
		return fmt.Errorf("handshake failed: %w", err)
	}

	// 启动读取循环
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

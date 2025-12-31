package client

import (
	"context"
	"net"
	"strings"
	"time"

	"tunnox-core/internal/client/transport"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"
)

// Connect 连接到服务器并建立指令连接
func (c *TunnoxClient) Connect() error {
	// 如果配置中没有地址且没有协议，使用自动连接
	// 注意：如果指定了协议但没有地址，应该报错而不是自动连接
	if c.config.Server.Address == "" && c.config.Server.Protocol == "" {
		return c.connectWithAutoDetection()
	}

	// 如果指定了协议但没有地址，报错
	if c.config.Server.Protocol != "" && c.config.Server.Address == "" {
		return coreerrors.Newf(coreerrors.CodeInvalidParam, "server address is required when protocol is specified (%s)", c.config.Server.Protocol)
	}

	corelog.Infof("Client: connecting to server %s", c.config.Server.Address)

	protocol := c.config.Server.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	corelog.Infof("Client: using %s transport for control connection", strings.ToUpper(protocol))

	// 1. 根据协议建立控制连接
	// 检查 context 是否已被取消
	select {
	case <-c.Ctx().Done():
		return coreerrors.Wrap(c.Ctx().Err(), coreerrors.CodeCancelled, "connection cancelled")
	default:
	}

	var (
		conn net.Conn
		err  error
	)

	// 为所有连接设置超时 context（20秒），并支持取消
	connectCtx, cancel := context.WithTimeout(c.Ctx(), 20*time.Second)
	defer cancel()

	// 启动 goroutine 监听 context 取消
	connectDone := make(chan struct {
		conn net.Conn
		err  error
	}, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				corelog.Errorf("Client: panic in connection goroutine: %v", r)
			}
		}()

		var resultConn net.Conn
		var resultErr error

		normalizedProtocol := strings.ToLower(protocol)

		// 检查协议是否可用
		if !transport.IsProtocolAvailable(normalizedProtocol) {
			availableProtocols := transport.GetAvailableProtocolNames()
			resultErr = coreerrors.Newf(coreerrors.CodeNotConfigured, "protocol %q is not available (compiled protocols: %v)", normalizedProtocol, availableProtocols)
		} else {
			// 使用统一的协议注册表拨号
			resultConn, resultErr = transport.Dial(connectCtx, normalizedProtocol, c.config.Server.Address)
			if resultErr == nil && normalizedProtocol == "tcp" {
				// 配置 TCP 连接选项
				SetKeepAliveIfSupported(resultConn, true)
			}
		}

		select {
		case connectDone <- struct {
			conn net.Conn
			err  error
		}{resultConn, resultErr}:
		case <-connectCtx.Done():
			// Context 已取消，关闭连接（如果已建立）
			if resultConn != nil {
				resultConn.Close()
			}
		}
	}()

	// 等待连接完成或 context 取消
	select {
	case result := <-connectDone:
		conn, err = result.conn, result.err
	case <-connectCtx.Done():
		err = coreerrors.Wrap(connectCtx.Err(), coreerrors.CodeCancelled, "connection cancelled")
	}

	if err != nil {
		return coreerrors.Wrapf(err, coreerrors.CodeNetworkError, "failed to dial server (%s)", protocol)
	}

	c.config.Server.Protocol = strings.ToLower(protocol)

	// 使用锁保护连接状态
	c.mu.Lock()
	c.controlConn = conn
	// 2. 创建 Stream
	streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
	c.controlStream = streamFactory.CreateStreamProcessor(conn, conn)
	c.mu.Unlock()

	// 记录连接信息用于调试
	localAddr := "unknown"
	remoteAddr := "unknown"
	if conn.LocalAddr() != nil {
		localAddr = conn.LocalAddr().String()
	}
	if conn.RemoteAddr() != nil {
		remoteAddr = conn.RemoteAddr().String()
	}
	corelog.Infof("Client: %s connection established - Local=%s, Remote=%s, controlStream=%p",
		strings.ToUpper(protocol), localAddr, remoteAddr, c.controlStream)

	// 3. 发送握手请求
	if err := c.sendHandshake(); err != nil {
		// 握手失败，清理连接资源
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
		return coreerrors.Wrap(err, coreerrors.CodeHandshakeFailed, "handshake failed")
	}

	// 4. 启动读取循环（接收服务器命令）
	// ✅ 防止重复启动 readLoop
	if !c.readLoopRunning.CompareAndSwap(false, true) {
		corelog.Warnf("Client: readLoop already running, skipping")
	} else {
		go func() {
			defer c.readLoopRunning.Store(false)
			c.readLoop()
		}()
	}

	// 5. 启动心跳循环
	// ✅ 防止重复启动 heartbeatLoop
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

// Disconnect 断开与服务器的连接
func (c *TunnoxClient) Disconnect() error {
	corelog.Infof("Client: disconnecting from server")

	// 使用锁保护连接状态
	c.mu.Lock()
	defer c.mu.Unlock()

	// 关闭控制流和连接
	if c.controlStream != nil {
		c.controlStream.Close()
		c.controlStream = nil
	}

	if c.controlConn != nil {
		c.controlConn.Close()
		c.controlConn = nil
	}

	corelog.Infof("Client: disconnected successfully")
	return nil
}

// IsConnected 检查是否连接到服务器
func (c *TunnoxClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.controlConn != nil && c.controlStream != nil
}

// Reconnect 重新连接到服务器
func (c *TunnoxClient) Reconnect() error {
	// ✅ 防止重复重连：如果已有重连在进行，直接返回
	if !c.reconnecting.CompareAndSwap(false, true) {
		corelog.Debugf("Client: reconnect already in progress, skipping Reconnect() call")
		return nil
	}
	defer c.reconnecting.Store(false)

	corelog.Infof("Client: attempting to reconnect...")

	// 先断开旧连接
	c.Disconnect()

	// 建立新连接
	return c.Connect()
}

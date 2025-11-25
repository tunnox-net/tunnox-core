package client

import (
	"context"
	"fmt"
	"net"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/stream/transform"
	"tunnox-core/internal/utils"
)

// TcpMappingHandler TCP 端口映射处理器
type TcpMappingHandler struct {
	*dispose.ManagerBase

	client   *TunnoxClient
	config   MappingConfig
	listener net.Listener
}

// NewTcpMappingHandler 创建 TCP 映射处理器
func NewTcpMappingHandler(client *TunnoxClient, config MappingConfig) *TcpMappingHandler {
	handler := &TcpMappingHandler{
		ManagerBase: dispose.NewManager(fmt.Sprintf("TcpMapping-%s", config.MappingID), client.Ctx()),
		client:      client,
		config:      config,
	}

	// 添加清理处理器
	handler.AddCleanHandler(func() error {
		utils.Infof("TcpMappingHandler[%s]: cleaning up resources", config.MappingID)
		if handler.listener != nil {
			handler.listener.Close()
		}
		return nil
	})

	return handler
}

// Start 启动 TCP 映射监听
func (h *TcpMappingHandler) Start() error {
	addr := fmt.Sprintf(":%d", h.config.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	h.listener = listener
	utils.Infof("TcpMappingHandler: listening on %s for mapping %s", addr, h.config.MappingID)

	// 启动接受连接的循环
	go h.acceptLoop()

	return nil
}

// acceptLoop 接受用户连接
func (h *TcpMappingHandler) acceptLoop() {
	for {
		select {
		case <-h.Ctx().Done():
			return
		default:
		}

		// 设置接受超时
		h.listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))

		userConn, err := h.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if h.Ctx().Err() != nil {
				return
			}
			utils.Errorf("TcpMappingHandler: failed to accept connection: %v", err)
			continue
		}

		// 处理用户连接
		go h.handleUserConnection(userConn)
	}
}

// handleUserConnection 处理用户连接
func (h *TcpMappingHandler) handleUserConnection(userConn net.Conn) {
	defer userConn.Close()

	utils.Infof("TcpMappingHandler: user connected from %s, establishing tunnel", userConn.RemoteAddr())

	// 1. 生成 TunnelID
	tunnelID := fmt.Sprintf("tcp-tunnel-%d-%d", time.Now().UnixNano(), h.config.LocalPort)

	// 2. 建立隧道连接
	tunnelConn, tunnelStream, err := h.client.DialTunnel(tunnelID, h.config.MappingID, h.config.SecretKey)
	if err != nil {
		utils.Errorf("TcpMappingHandler: failed to dial tunnel: %v", err)
		return
	}
	defer tunnelConn.Close()

	utils.Infof("TcpMappingHandler: tunnel %s established successfully", tunnelID)

	// 3. 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 4. 创建转换器并启动双向转发
	transformer, err := h.createTransformer()
	if err != nil {
		utils.Errorf("TcpMappingHandler: failed to create transformer: %v", err)
		return
	}

	utils.BidirectionalCopy(userConn, tunnelConn, &utils.BidirectionalCopyOptions{
		Transformer: transformer,
		LogPrefix:   fmt.Sprintf("TcpMappingHandler[%s]", tunnelID),
	})
}

// createTransformer 创建流转换器
func (h *TcpMappingHandler) createTransformer() (transform.StreamTransformer, error) {
	config := &transform.TransformConfig{
		EnableCompression: h.config.EnableCompression,
		CompressionLevel:  h.config.CompressionLevel,
		EnableEncryption:  h.config.EnableEncryption,
		EncryptionMethod:  h.config.EncryptionMethod,
		EncryptionKey:     h.config.EncryptionKey,
	}

	return transform.NewTransformer(config)
}

// Stop 停止映射处理器
func (h *TcpMappingHandler) Stop() {
	utils.Infof("TcpMappingHandler: stopping mapping %s", h.config.MappingID)
	h.Close()
}

// GetConfig 返回映射配置
func (h *TcpMappingHandler) GetConfig() MappingConfig {
	return h.config
}

// GetContext 返回上下文
func (h *TcpMappingHandler) GetContext() context.Context {
	return h.Ctx()
}

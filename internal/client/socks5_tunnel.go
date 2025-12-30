// Package client SOCKS5 隧道创建
// 实现 ClientA 端的 SOCKS5 隧道创建逻辑
package client

import (
	"fmt"
	"io"
	"net"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// SOCKS5TunnelCreatorImpl SOCKS5 隧道创建器实现
type SOCKS5TunnelCreatorImpl struct {
	client      *TunnoxClient
	idGenerator func() string
}

// NewSOCKS5TunnelCreatorImpl 创建 SOCKS5 隧道创建器
func NewSOCKS5TunnelCreatorImpl(client *TunnoxClient) *SOCKS5TunnelCreatorImpl {
	return &SOCKS5TunnelCreatorImpl{
		client:      client,
		idGenerator: generateSOCKS5TunnelID,
	}
}

// generateSOCKS5TunnelID 生成 SOCKS5 隧道ID
func generateSOCKS5TunnelID() string {
	return fmt.Sprintf("socks5-%d", time.Now().UnixNano())
}

// CreateSOCKS5Tunnel 创建 SOCKS5 隧道
// 实现 SOCKS5TunnelCreator 接口
func (c *SOCKS5TunnelCreatorImpl) CreateSOCKS5Tunnel(
	userConn net.Conn,
	mappingID string,
	targetClientID int64,
	targetHost string,
	targetPort int,
	secretKey string,
) error {
	// 1. 生成隧道ID
	tunnelID := c.idGenerator()

	// 2. 建立到 Server 的隧道连接（包含 SOCKS5 动态目标地址）
	// 重要：必须使用 tunnelStream 进行数据传输，而不是直接使用 serverConn
	// 因为 serverConn 已经被 tunnelStream 包装，并且已经执行了握手和 TunnelOpen 协议
	serverConn, tunnelStream, err := c.client.dialTunnelWithTarget(tunnelID, mappingID, secretKey, targetHost, targetPort)
	if err != nil {
		return fmt.Errorf("failed to dial tunnel: %w", err)
	}

	// 3. 开始双向数据转发
	go c.forwardData(tunnelID, userConn, serverConn, tunnelStream)

	return nil
}

// forwardData 双向数据转发
// 使用 SimpleBidirectionalCopy 实现正确的半关闭语义，
// 解决 HTTP 请求发送完毕后响应数据无法传回的问题
func (c *SOCKS5TunnelCreatorImpl) forwardData(tunnelID string, userConn, serverConn net.Conn, tunnelStream stream.PackageStreamer) {
	logPrefix := fmt.Sprintf("SOCKS5Tunnel[%s]", tunnelID)

	// 从 tunnelStream 获取 Reader/Writer（支持压缩/加密）
	tunnelReader := tunnelStream.GetReader()
	tunnelWriter := tunnelStream.GetWriter()

	// 如果 GetReader/GetWriter 返回 nil，回退到直接使用 serverConn
	if tunnelReader == nil {
		tunnelReader = serverConn
	}
	if tunnelWriter == nil {
		tunnelWriter = serverConn
	}

	// 包装成 ReadWriteCloser（确保关闭时同时关闭 stream 和 conn）
	tunnelRWC, err := utils.NewReadWriteCloser(tunnelReader, tunnelWriter, func() error {
		tunnelStream.Close()
		serverConn.Close()
		return nil
	})
	if err != nil {
		corelog.Errorf("SOCKS5Tunnel[%s]: failed to create tunnel ReadWriteCloser: %v", tunnelID, err)
		serverConn.Close()
		userConn.Close()
		return
	}

	result := utils.SimpleBidirectionalCopy(userConn, tunnelRWC, logPrefix)

	if result.SendError != nil && result.SendError != io.EOF {
		corelog.Warnf("SOCKS5Tunnel[%s]: user->server error: %v", tunnelID, result.SendError)
	}
	if result.ReceiveError != nil && result.ReceiveError != io.EOF {
		corelog.Warnf("SOCKS5Tunnel[%s]: server->user error: %v", tunnelID, result.ReceiveError)
	}

	corelog.Debugf("SOCKS5Tunnel[%s]: completed, sent=%d, received=%d",
		tunnelID, result.BytesSent, result.BytesReceived)
}

// SOCKS5TunnelRequest SOCKS5 隧道请求
type SOCKS5TunnelRequest struct {
	TunnelID       string `json:"tunnel_id"`
	MappingID      string `json:"mapping_id"`
	TargetClientID int64  `json:"target_client_id"`
	TargetHost     string `json:"target_host"`
	TargetPort     int    `json:"target_port"`
	Protocol       string `json:"protocol"`
}

// SetIDGenerator 设置ID生成器（用于测试）
func (c *SOCKS5TunnelCreatorImpl) SetIDGenerator(gen func() string) {
	c.idGenerator = gen
}

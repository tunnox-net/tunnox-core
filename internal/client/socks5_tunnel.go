// Package client SOCKS5 隧道创建
// 实现 ClientA 端的 SOCKS5 隧道创建逻辑
package client

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	corelog "tunnox-core/internal/core/log"
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
	serverConn, _, err := c.client.dialTunnelWithTarget(tunnelID, mappingID, secretKey, targetHost, targetPort)
	if err != nil {
		return fmt.Errorf("failed to dial tunnel: %w", err)
	}


	// 3. 开始双向数据转发
	go c.forwardData(tunnelID, userConn, serverConn)

	return nil
}

// forwardData 双向数据转发
func (c *SOCKS5TunnelCreatorImpl) forwardData(tunnelID string, userConn, serverConn net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// 用户 -> 服务器
	go func() {
		defer wg.Done()
		_, err := io.Copy(serverConn, userConn)
		if err != nil && err != io.EOF {
			corelog.Debugf("SOCKS5Tunnel[%s]: user->server error: %v", tunnelID, err)
		}
		serverConn.Close()
	}()

	// 服务器 -> 用户
	go func() {
		defer wg.Done()
		_, err := io.Copy(userConn, serverConn)
		if err != nil && err != io.EOF {
			corelog.Debugf("SOCKS5Tunnel[%s]: server->user error: %v", tunnelID, err)
		}
		userConn.Close()
	}()

	wg.Wait()
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

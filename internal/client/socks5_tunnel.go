// Package client SOCKS5 隧道创建
// 实现 ClientA 端的 SOCKS5 隧道创建逻辑
package client

import (
	"fmt"
	"io"
	"net"
	"time"

	"tunnox-core/internal/client/socks5"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils/iocopy"
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
// onSuccess 回调在隧道建立成功后、数据转发开始前调用
func (c *SOCKS5TunnelCreatorImpl) CreateSOCKS5Tunnel(
	userConn net.Conn,
	mappingID string,
	targetClientID int64,
	targetHost string,
	targetPort int,
	secretKey string,
	onSuccess func(),
) error {
	// 1. 生成隧道ID
	tunnelID := c.idGenerator()

	// 2. 建立到 Server 的隧道连接（包含 SOCKS5 动态目标地址）
	// 重要：必须使用 tunnelStream 进行数据传输，而不是直接使用 serverConn
	// 因为 serverConn 已经被 tunnelStream 包装，并且已经执行了握手和 TunnelOpen 协议
	serverConn, tunnelStream, err := c.client.dialTunnelWithTarget(tunnelID, mappingID, secretKey, targetHost, targetPort)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to dial tunnel")
	}

	// 3. 隧道建立成功，发送 SOCKS5 成功响应
	// 必须在数据转发开始前发送，否则浏览器不会发送 HTTP 请求
	if onSuccess != nil {
		onSuccess()
	}

	// 4. 开始双向数据转发
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
	tunnelRWC, err := iocopy.NewReadWriteCloser(tunnelReader, tunnelWriter, func() error {
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

	result := iocopy.Simple(userConn, tunnelRWC, logPrefix)

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

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// UDP 隧道支持（用于 SOCKS5 UDP ASSOCIATE）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// UDPRelayCreatorImpl 实现 socks5.UDPRelayCreator 接口
type UDPRelayCreatorImpl struct {
	client *TunnoxClient
}

func NewUDPRelayCreatorImpl(client *TunnoxClient) *UDPRelayCreatorImpl {
	return &UDPRelayCreatorImpl{client: client}
}

func (c *UDPRelayCreatorImpl) CreateUDPRelay(
	tcpConn net.Conn,
	mappingID string,
	targetClientID int64,
	secretKey string,
) (*net.UDPAddr, error) {
	config := &socks5.UDPRelayConfig{
		MappingID:      mappingID,
		TargetClientID: targetClientID,
		SecretKey:      secretKey,
		BindAddr:       "127.0.0.1:0",
	}

	tunnelCreator := &UDPTunnelCreatorImpl{client: c.client}

	relay, err := socks5.NewUDPRelay(c.client.Ctx(), tcpConn, config, tunnelCreator)
	if err != nil {
		return nil, err
	}

	// 设置 DNS 查询处理器，将 DNS 请求通过控制通道转发（而非不稳定的 UDP 隧道）
	// TunnoxClient 实现了 socks5.DNSQueryHandler 接口
	relay.SetDNSHandler(c.client)
	corelog.Infof("UDPRelayCreator: DNS handler set for mapping %s (via control channel)", mappingID)

	return relay.GetBindAddr(), nil
}

// UDPTunnelCreatorImpl 实现 socks5.UDPTunnelCreator 接口
type UDPTunnelCreatorImpl struct {
	client *TunnoxClient
}

func (c *UDPTunnelCreatorImpl) CreateUDPTunnel(
	mappingID string,
	targetClientID int64,
	targetHost string,
	targetPort int,
	secretKey string,
) (socks5.UDPTunnelConn, error) {
	tunnelID := fmt.Sprintf("udp-%d", time.Now().UnixNano())

	serverConn, tunnelStream, err := c.client.dialTunnelWithTargetNetwork(
		tunnelID, mappingID, secretKey, targetHost, targetPort, "udp")
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to dial UDP tunnel")
	}

	conn := &udpTunnelConn{
		tunnelID:     tunnelID,
		serverConn:   serverConn,
		tunnelStream: tunnelStream,
	}

	corelog.Debugf("UDPTunnelCreator: created tunnel %s for %s:%d", tunnelID, targetHost, targetPort)
	return conn, nil
}

// udpTunnelConn 实现 socks5.UDPTunnelConn 接口
type udpTunnelConn struct {
	tunnelID     string
	serverConn   net.Conn
	tunnelStream stream.PackageStreamer
}

func (c *udpTunnelConn) SendPacket(data []byte) error {
	writer := c.tunnelStream.GetWriter()
	if writer == nil {
		writer = c.serverConn
	}

	// 长度前缀协议: 2字节长度 + 数据
	lenBuf := make([]byte, 2)
	lenBuf[0] = byte(len(data) >> 8)
	lenBuf[1] = byte(len(data) & 0xFF)

	if _, err := writer.Write(lenBuf); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write packet length")
	}
	if _, err := writer.Write(data); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write packet data")
	}

	return nil
}

func (c *udpTunnelConn) ReceivePacket() ([]byte, error) {
	reader := c.tunnelStream.GetReader()
	if reader == nil {
		reader = c.serverConn
	}

	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read packet length")
	}

	packetLen := int(lenBuf[0])<<8 | int(lenBuf[1])
	if packetLen > 65535 {
		return nil, coreerrors.Newf(coreerrors.CodeProtocolError, "packet too large: %d", packetLen)
	}

	data := make([]byte, packetLen)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read packet data")
	}

	return data, nil
}

func (c *udpTunnelConn) Close() error {
	corelog.Debugf("UDPTunnelConn[%s]: closing", c.tunnelID)
	c.tunnelStream.Close()
	return c.serverConn.Close()
}

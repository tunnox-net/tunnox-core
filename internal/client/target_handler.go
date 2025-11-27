package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/protocol/udp"
	"tunnox-core/internal/stream/transform"
	"tunnox-core/internal/utils"
)

// handleTunnelOpenRequest 处理隧道打开请求（作为目标客户端）
func (c *TunnoxClient) handleTunnelOpenRequest(cmdBody string) {
	var req struct {
		TunnelID   string `json:"tunnel_id"`
		MappingID  string `json:"mapping_id"`
		SecretKey  string `json:"secret_key"`
		TargetHost string `json:"target_host"`
		TargetPort int    `json:"target_port"`
		Protocol   string `json:"protocol"` // tcp/udp/socks5

		EnableCompression bool   `json:"enable_compression"`
		CompressionLevel  int    `json:"compression_level"`
		EnableEncryption  bool   `json:"enable_encryption"`
		EncryptionMethod  string `json:"encryption_method"`
		EncryptionKey     string `json:"encryption_key"` // hex编码
		BandwidthLimit    int64  `json:"bandwidth_limit"`
	}

	if err := json.Unmarshal([]byte(cmdBody), &req); err != nil {
		utils.Errorf("Client: failed to parse tunnel open request: %v", err)
		return
	}

	// 创建Transform配置（只用于限速）
	transformConfig := &transform.TransformConfig{
		BandwidthLimit: req.BandwidthLimit,
	}

	// 根据协议类型分发
	protocol := req.Protocol
	if protocol == "" {
		protocol = "tcp" // 默认 TCP
	}

	switch protocol {
	case "tcp":
		go c.handleTCPTargetTunnel(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
	case "udp":
		go c.handleUDPTargetTunnel(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
	case "socks5":
		go c.handleSOCKS5TargetTunnel(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
	default:
		utils.Errorf("Client: unsupported protocol: %s", protocol)
	}
}

// handleTCPTargetTunnel 处理TCP目标端隧道
func (c *TunnoxClient) handleTCPTargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int,
	transformConfig *transform.TransformConfig) {
	// 1. 连接到目标服务
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		utils.Errorf("Client: failed to connect to target %s: %v", targetAddr, err)
		return
	}
	defer targetConn.Close()

	// 2. 建立隧道连接
	tunnelConn, tunnelStream, err := c.dialTunnel(tunnelID, mappingID, secretKey)
	if err != nil {
		utils.Errorf("Client: failed to dial tunnel: %v", err)
		return
	}
	defer tunnelConn.Close()

	utils.Infof("Client: TCP tunnel %s established for target %s", tunnelID, targetAddr)

	// 3. 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 4. 创建转换器并启动双向转发
	transformer, _ := transform.NewTransformer(transformConfig)
	utils.BidirectionalCopy(targetConn, tunnelConn, &utils.BidirectionalCopyOptions{
		Transformer: transformer,
		LogPrefix:   fmt.Sprintf("Client[TCP-target][%s]", tunnelID),
	})
}

// handleSOCKS5TargetTunnel 处理SOCKS5目标端隧道（与TCP流程一致）
func (c *TunnoxClient) handleSOCKS5TargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int,
	transformConfig *transform.TransformConfig) {
	utils.Infof("Client: handling SOCKS5 target tunnel, tunnel_id=%s, target=%s:%d", tunnelID, targetHost, targetPort)
	c.handleTCPTargetTunnel(tunnelID, mappingID, secretKey, targetHost, targetPort, transformConfig)
}

// handleUDPTargetTunnel 处理UDP目标端隧道
func (c *TunnoxClient) handleUDPTargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int,
	transformConfig *transform.TransformConfig) {
	utils.Infof("Client: handling UDP target tunnel, tunnel_id=%s, target=%s:%d", tunnelID, targetHost, targetPort)

	// 1. 解析目标 UDP 地址
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	udpAddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		utils.Errorf("Client: failed to resolve UDP address %s: %v", targetAddr, err)
		return
	}

	// 2. 创建 UDP 连接到目标
	targetConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		utils.Errorf("Client: failed to connect to UDP target %s: %v", targetAddr, err)
		return
	}
	defer targetConn.Close()

	utils.Infof("Client: connected to UDP target %s for tunnel %s", targetAddr, tunnelID)

	// 3. 建立隧道连接
	tunnelConn, tunnelStream, err := c.dialTunnel(tunnelID, mappingID, secretKey)
	if err != nil {
		utils.Errorf("Client: failed to dial tunnel: %v", err)
		return
	}
	defer tunnelConn.Close()

	utils.Infof("Client: UDP tunnel %s established successfully", tunnelID)

	// 4. 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 5. 启动 UDP 双向转发
	c.bidirectionalCopyUDPTarget(tunnelConn, targetConn, tunnelID, transformConfig)
}

// bidirectionalCopyUDPTarget UDP 双向转发（作为目标端）
func (c *TunnoxClient) bidirectionalCopyUDPTarget(tunnelConn net.Conn, targetConn *net.UDPConn, tunnelID string, transformConfig *transform.TransformConfig) {
	// 创建转换器
	transformer, err := transform.NewTransformer(transformConfig)
	if err != nil {
		utils.Errorf("Client: failed to create transformer: %v", err)
		return
	}

	// 包装读写器
	reader := io.Reader(tunnelConn)
	writer := io.Writer(tunnelConn)
	if transformer != nil {
		reader, _ = transformer.WrapReader(reader)
		writer, _ = transformer.WrapWriter(writer)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// 从隧道读取数据并发送到目标 UDP
	go func() {
		defer wg.Done()
		for {
			data, err := udp.ReadLengthPrefixedPacket(reader)
			if err != nil {
				if err != io.EOF {
					utils.Errorf("UDPTarget[%s]: failed to read length from tunnel: %v", tunnelID, err)
				}
				return
			}

			// 发送到目标 UDP
			_, err = targetConn.Write(data)
			if err != nil {
				utils.Errorf("UDPTarget[%s]: failed to write to target: %v", tunnelID, err)
				return
			}
		}
	}()

	// 从目标 UDP 读取数据并发送到隧道
	go func() {
		defer wg.Done()
		buf := make([]byte, 65535)
		targetConn.SetReadDeadline(time.Now().Add(60 * time.Second))

		for {
			n, err := targetConn.Read(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					utils.Debugf("UDPTarget[%s]: read timeout, closing tunnel", tunnelID)
				} else {
					utils.Errorf("UDPTarget[%s]: failed to read from target: %v", tunnelID, err)
				}
				return
			}

			if n > 0 {
				// 重置超时
				targetConn.SetReadDeadline(time.Now().Add(60 * time.Second))

				if err := udp.WriteLengthPrefixedPacket(writer, buf[:n]); err != nil {
					utils.Errorf("UDPTarget[%s]: failed to write data to tunnel: %v", tunnelID, err)
					return
				}
			}
		}
	}()

	wg.Wait()
	utils.Infof("UDPTarget[%s]: tunnel closed", tunnelID)
}


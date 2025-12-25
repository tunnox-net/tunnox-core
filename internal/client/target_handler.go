package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"

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
		corelog.Errorf("Client: failed to parse tunnel open request: %v", err)
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
		corelog.Errorf("Client: unsupported protocol: %s", protocol)
	}
}

// handleTCPTargetTunnel 处理TCP目标端隧道
func (c *TunnoxClient) handleTCPTargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int,
	transformConfig *transform.TransformConfig) {
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	corelog.Infof("Client[TCP-target][%s]: connecting to target %s and dialing tunnel in parallel", tunnelID, targetAddr)

	// 并行执行：连接目标服务 和 建立隧道连接
	type targetResult struct {
		conn net.Conn
		err  error
	}
	type tunnelResult struct {
		conn   net.Conn
		stream stream.PackageStreamer
		err    error
	}

	targetCh := make(chan targetResult, 1)
	tunnelCh := make(chan tunnelResult, 1)

	// 1. 并行连接目标服务
	go func() {
		conn, err := net.DialTimeout("tcp", targetAddr, 30*time.Second)
		targetCh <- targetResult{conn: conn, err: err}
	}()

	// 2. 并行建立隧道连接
	go func() {
		conn, stream, err := c.dialTunnel(tunnelID, mappingID, secretKey)
		tunnelCh <- tunnelResult{conn: conn, stream: stream, err: err}
	}()

	// 等待两个操作完成
	var targetConn net.Conn
	var tunnelConn net.Conn
	var tunnelStream stream.PackageStreamer

	for i := 0; i < 2; i++ {
		select {
		case tr := <-targetCh:
			if tr.err != nil {
				corelog.Errorf("Client[TCP-target][%s]: failed to connect to target %s: %v", tunnelID, targetAddr, tr.err)
				// 等待隧道结果并关闭
				select {
				case tunRes := <-tunnelCh:
					if tunRes.conn != nil {
						tunRes.conn.Close()
					}
				case <-time.After(5 * time.Second):
				}
				return
			}
			targetConn = tr.conn
		case tunRes := <-tunnelCh:
			if tunRes.err != nil {
				corelog.Errorf("Client[TCP-target][%s]: failed to dial tunnel: %v", tunnelID, tunRes.err)
				// 等待目标连接结果并关闭
				select {
				case tgtRes := <-targetCh:
					if tgtRes.conn != nil {
						tgtRes.conn.Close()
					}
				case <-time.After(5 * time.Second):
				}
				return
			}
			tunnelConn = tunRes.conn
			tunnelStream = tunRes.stream
		}
	}

	defer targetConn.Close()
	defer tunnelConn.Close()

	corelog.Infof("Client[TCP-target][%s]: tunnel established for target %s", tunnelID, targetAddr)

	// 3. 通过接口抽象获取 Reader/Writer（不依赖具体协议）
	// 优先使用 tunnelStream 的 Reader/Writer（支持压缩/加密）
	// 如果没有，则使用 tunnelConn（通过接口抽象，不依赖具体类型）
	tunnelReader := tunnelStream.GetReader()
	tunnelWriter := tunnelStream.GetWriter()

	// 如果 GetReader/GetWriter 返回 nil，尝试使用 tunnelConn（通过接口抽象）
	if tunnelReader == nil {
		if tunnelConn != nil {
			// tunnelConn 实现了 io.Reader（通过接口抽象）
			if reader, ok := tunnelConn.(io.Reader); ok && reader != nil {
				tunnelReader = reader
			} else {
				corelog.Errorf("Client[TCP-target][%s]: tunnelConn does not implement io.Reader or reader is nil, GetReader() returned nil", tunnelID)
				return
			}
		} else {
			corelog.Errorf("Client[TCP-target][%s]: tunnelConn is nil and GetReader() returned nil", tunnelID)
			return
		}
	}
	if tunnelWriter == nil {
		if tunnelConn != nil {
			// tunnelConn 实现了 io.Writer（通过接口抽象）
			if writer, ok := tunnelConn.(io.Writer); ok && writer != nil {
				tunnelWriter = writer
			} else {
				corelog.Errorf("Client[TCP-target][%s]: tunnelConn does not implement io.Writer or writer is nil, GetWriter() returned nil", tunnelID)
				return
			}
		} else {
			corelog.Errorf("Client[TCP-target][%s]: tunnelConn is nil and GetWriter() returned nil", tunnelID)
			return
		}
	}

	// 4. 包装隧道连接成 ReadWriteCloser（确保关闭时同时关闭 stream 和 conn）
	// 添加额外的 nil 检查，确保不会传入 nil
	if tunnelReader == nil || tunnelWriter == nil {
		corelog.Errorf("Client[TCP-target][%s]: tunnelReader or tunnelWriter is nil after setup, reader=%v, writer=%v", tunnelID, tunnelReader != nil, tunnelWriter != nil)
		return
	}
	tunnelRWC := utils.NewReadWriteCloser(tunnelReader, tunnelWriter, func() error {
		tunnelStream.Close()
		if tunnelConn != nil {
			tunnelConn.Close()
		}
		return nil
	})

	// 5. 创建转换器并启动双向转发
	transformer, _ := transform.NewTransformer(transformConfig)
	utils.BidirectionalCopy(targetConn, tunnelRWC, &utils.BidirectionalCopyOptions{
		Transformer: transformer,
		LogPrefix:   fmt.Sprintf("Client[TCP-target][%s]", tunnelID),
	})
}

// handleSOCKS5TargetTunnel 处理SOCKS5目标端隧道（与TCP流程一致）
func (c *TunnoxClient) handleSOCKS5TargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int,
	transformConfig *transform.TransformConfig) {
	corelog.Infof("Client: handling SOCKS5 target tunnel, tunnel_id=%s, target=%s:%d", tunnelID, targetHost, targetPort)
	c.handleTCPTargetTunnel(tunnelID, mappingID, secretKey, targetHost, targetPort, transformConfig)
}

// handleUDPTargetTunnel 处理UDP目标端隧道
func (c *TunnoxClient) handleUDPTargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int,
	transformConfig *transform.TransformConfig) {
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	corelog.Infof("Client[UDP-target][%s]: connecting to target %s and dialing tunnel in parallel", tunnelID, targetAddr)

	// 并行执行：连接目标服务 和 建立隧道连接
	type targetResult struct {
		conn *net.UDPConn
		err  error
	}
	type tunnelResult struct {
		conn   net.Conn
		stream stream.PackageStreamer
		err    error
	}

	targetCh := make(chan targetResult, 1)
	tunnelCh := make(chan tunnelResult, 1)

	// 1. 并行连接目标服务
	go func() {
		udpAddr, err := net.ResolveUDPAddr("udp", targetAddr)
		if err != nil {
			targetCh <- targetResult{err: fmt.Errorf("failed to resolve UDP address: %w", err)}
			return
		}
		conn, err := net.DialUDP("udp", nil, udpAddr)
		targetCh <- targetResult{conn: conn, err: err}
	}()

	// 2. 并行建立隧道连接
	go func() {
		conn, stream, err := c.dialTunnel(tunnelID, mappingID, secretKey)
		tunnelCh <- tunnelResult{conn: conn, stream: stream, err: err}
	}()

	// 等待两个操作完成
	var targetConn *net.UDPConn
	var tunnelConn net.Conn
	var tunnelStream stream.PackageStreamer

	for i := 0; i < 2; i++ {
		select {
		case tr := <-targetCh:
			if tr.err != nil {
				corelog.Errorf("Client[UDP-target][%s]: failed to connect to target %s: %v", tunnelID, targetAddr, tr.err)
				select {
				case tunRes := <-tunnelCh:
					if tunRes.conn != nil {
						tunRes.conn.Close()
					}
				case <-time.After(5 * time.Second):
				}
				return
			}
			targetConn = tr.conn
		case tunRes := <-tunnelCh:
			if tunRes.err != nil {
				corelog.Errorf("Client[UDP-target][%s]: failed to dial tunnel: %v", tunnelID, tunRes.err)
				select {
				case tgtRes := <-targetCh:
					if tgtRes.conn != nil {
						tgtRes.conn.Close()
					}
				case <-time.After(5 * time.Second):
				}
				return
			}
			tunnelConn = tunRes.conn
			tunnelStream = tunRes.stream
		}
	}

	defer targetConn.Close()
	defer tunnelConn.Close()

	corelog.Infof("Client[UDP-target][%s]: tunnel established for target %s", tunnelID, targetAddr)

	// 3. 获取 Reader/Writer
	tunnelReader := tunnelStream.GetReader()
	tunnelWriter := tunnelStream.GetWriter()

	if tunnelReader == nil {
		if tunnelConn != nil {
			if reader, ok := tunnelConn.(io.Reader); ok && reader != nil {
				tunnelReader = reader
			} else {
				corelog.Errorf("Client[UDP-target][%s]: tunnelConn does not implement io.Reader", tunnelID)
				return
			}
		} else {
			corelog.Errorf("Client[UDP-target][%s]: tunnelConn is nil and GetReader() returned nil", tunnelID)
			return
		}
	}

	if tunnelWriter == nil {
		if tunnelConn != nil {
			if writer, ok := tunnelConn.(io.Writer); ok && writer != nil {
				tunnelWriter = writer
			} else {
				corelog.Errorf("Client[UDP-target][%s]: tunnelConn does not implement io.Writer", tunnelID)
				return
			}
		} else {
			corelog.Errorf("Client[UDP-target][%s]: tunnelConn is nil and GetWriter() returned nil", tunnelID)
			return
		}
	}

	// 4. 包装隧道连接
	if tunnelReader == nil || tunnelWriter == nil {
		corelog.Errorf("Client[UDP-target][%s]: tunnelReader or tunnelWriter is nil", tunnelID)
		return
	}

	tunnelRWC := utils.NewReadWriteCloser(tunnelReader, tunnelWriter, func() error {
		tunnelStream.Close()
		if tunnelConn != nil {
			tunnelConn.Close()
		}
		return nil
	})

	// 5. 双向转发（UDP需要特殊处理数据包边界）
	transformer, _ := transform.NewTransformer(transformConfig)
	utils.BidirectionalCopy(targetConn, tunnelRWC, &utils.BidirectionalCopyOptions{
		Transformer: transformer,
		LogPrefix:   fmt.Sprintf("Client[UDP-target][%s]", tunnelID),
	})
}

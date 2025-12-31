package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
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
	const logPrefix = "Client[TCP-target]"
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	corelog.Infof("%s[%s]: connecting to target %s and dialing tunnel in parallel", logPrefix, tunnelID, targetAddr)

	// 注册隧道到管理器（用于接收关闭通知时关闭隧道）
	tunnelCtx, tunnelCancel := c.targetTunnelManager.RegisterTunnel(tunnelID, c.Ctx())
	defer func() {
		tunnelCancel()
		c.targetTunnelManager.UnregisterTunnel(tunnelID)
	}()

	// 并行执行：连接目标服务 和 建立隧道连接
	targetCh := make(chan tcpTargetResult, 1)
	tunnelCh := make(chan tunnelResult, 1)

	// 1. 并行连接目标服务
	go func() {
		conn, err := net.DialTimeout("tcp", targetAddr, 30*time.Second)
		targetCh <- tcpTargetResult{conn: conn, err: err}
	}()

	// 2. 并行建立隧道连接
	go func() {
		conn, tunnelStream, err := c.dialTunnel(tunnelID, mappingID, secretKey)
		tunnelCh <- tunnelResult{conn: conn, stream: tunnelStream, err: err}
	}()

	// 等待两个操作完成
	targetConn, tunnelConn, tunnelStream, ok := waitForTCPConnections(
		tunnelCtx, targetCh, tunnelCh, logPrefix, tunnelID, targetAddr)
	if !ok {
		return
	}

	defer targetConn.Close()
	defer tunnelConn.Close()

	corelog.Infof("%s[%s]: tunnel established for target %s", logPrefix, tunnelID, targetAddr)

	// 监听隧道取消信号，收到时关闭连接以中断数据传输
	startCancellationWatcher(tunnelCtx, logPrefix, tunnelID, targetConn, tunnelConn)

	// 3. 获取 Reader/Writer
	tunnelReader, tunnelWriter, ok := getTunnelReaderWriter(tunnelStream, tunnelConn, logPrefix, tunnelID)
	if !ok {
		return
	}

	// 4. 包装隧道连接成 ReadWriteCloser
	tunnelRWC, ok := createTunnelRWC(tunnelReader, tunnelWriter, tunnelStream, tunnelConn, logPrefix, tunnelID)
	if !ok {
		return
	}

	// 5. 创建转换器并启动双向转发
	corelog.Infof("%s[%s]: starting BidirectionalCopy, targetConn=%T, tunnelRWC=%T", logPrefix, tunnelID, targetConn, tunnelRWC)
	startTime := time.Now()

	transformer, _ := transform.NewTransformer(transformConfig)
	result := utils.BidirectionalCopy(targetConn, tunnelRWC, &utils.BidirectionalCopyOptions{
		Transformer: transformer,
		LogPrefix:   fmt.Sprintf("%s[%s]", logPrefix, tunnelID),
	})

	elapsed := time.Since(startTime)
	corelog.Infof("%s[%s]: BidirectionalCopy completed after %v, sent=%d, recv=%d, sendErr=%v, recvErr=%v",
		logPrefix, tunnelID, elapsed, result.BytesSent, result.BytesReceived, result.SendError, result.ReceiveError)
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
	const logPrefix = "Client[UDP-target]"
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	corelog.Infof("%s[%s]: connecting to target %s and dialing tunnel in parallel", logPrefix, tunnelID, targetAddr)

	// 注册隧道到管理器（用于接收关闭通知时关闭隧道）
	tunnelCtx, tunnelCancel := c.targetTunnelManager.RegisterTunnel(tunnelID, c.Ctx())
	defer func() {
		tunnelCancel()
		c.targetTunnelManager.UnregisterTunnel(tunnelID)
	}()

	// 并行执行：连接目标服务 和 建立隧道连接
	targetCh := make(chan udpTargetResult, 1)
	tunnelCh := make(chan tunnelResult, 1)

	// 1. 并行连接目标服务
	go func() {
		udpAddr, err := net.ResolveUDPAddr("udp", targetAddr)
		if err != nil {
			targetCh <- udpTargetResult{err: coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to resolve UDP address")}
			return
		}
		conn, err := net.DialUDP("udp", nil, udpAddr)
		targetCh <- udpTargetResult{conn: conn, err: err}
	}()

	// 2. 并行建立隧道连接
	go func() {
		conn, tunnelStream, err := c.dialTunnel(tunnelID, mappingID, secretKey)
		tunnelCh <- tunnelResult{conn: conn, stream: tunnelStream, err: err}
	}()

	// 等待两个操作完成
	targetConn, tunnelConn, tunnelStream, ok := waitForUDPConnections(
		tunnelCtx, targetCh, tunnelCh, logPrefix, tunnelID, targetAddr)
	if !ok {
		return
	}

	defer targetConn.Close()
	defer tunnelConn.Close()

	corelog.Infof("%s[%s]: tunnel established for target %s", logPrefix, tunnelID, targetAddr)

	// 监听隧道取消信号，收到时关闭连接以中断数据传输
	startCancellationWatcher(tunnelCtx, logPrefix, tunnelID, targetConn, tunnelConn)

	// 3. 获取 Reader/Writer
	tunnelReader, tunnelWriter, ok := getTunnelReaderWriter(tunnelStream, tunnelConn, logPrefix, tunnelID)
	if !ok {
		return
	}

	// 4. 包装隧道连接成 ReadWriteCloser
	tunnelRWC, ok := createTunnelRWC(tunnelReader, tunnelWriter, tunnelStream, tunnelConn, logPrefix, tunnelID)
	if !ok {
		return
	}

	// 5. 双向转发（UDP需要特殊处理数据包边界）
	utils.UDPBidirectionalCopy(targetConn, tunnelRWC, &utils.BidirectionalCopyOptions{
		LogPrefix: fmt.Sprintf("%s[%s]", logPrefix, tunnelID),
	})
}

// tcpTargetResult TCP目标连接结果
type tcpTargetResult struct {
	conn net.Conn
	err  error
}

// udpTargetResult UDP目标连接结果
type udpTargetResult struct {
	conn *net.UDPConn
	err  error
}

// tunnelResult 隧道连接结果
type tunnelResult struct {
	conn   net.Conn
	stream stream.PackageStreamer
	err    error
}

// targetResultHandler 通用目标连接结果处理器
type targetResultHandler[T io.Closer] struct {
	conn T
	err  error
}

// waitForConnectionsGeneric 通用连接等待逻辑
// 使用泛型消除 TCP/UDP 连接等待的重复代码
func waitForConnectionsGeneric[T io.Closer](
	tunnelCtx context.Context,
	targetCh <-chan targetResultHandler[T],
	tunnelCh <-chan tunnelResult,
	logPrefix, tunnelID, targetAddr string,
) (T, net.Conn, stream.PackageStreamer, bool) {
	var targetConn T
	var tunnelConn net.Conn
	var tunnelStream stream.PackageStreamer
	var zeroT T

	for i := 0; i < 2; i++ {
		select {
		case <-tunnelCtx.Done():
			corelog.Infof("%s[%s]: tunnel cancelled before connection established", logPrefix, tunnelID)
			cleanupPendingConnections(targetCh, tunnelCh)
			return zeroT, nil, nil, false

		case tr := <-targetCh:
			if tr.err != nil {
				corelog.Errorf("%s[%s]: failed to connect to target %s: %v", logPrefix, tunnelID, targetAddr, tr.err)
				cleanupTunnelConnection(tunnelCh)
				return zeroT, nil, nil, false
			}
			targetConn = tr.conn

		case tunRes := <-tunnelCh:
			if tunRes.err != nil {
				corelog.Errorf("%s[%s]: failed to dial tunnel: %v", logPrefix, tunnelID, tunRes.err)
				cleanupTargetConnection(targetCh)
				return zeroT, nil, nil, false
			}
			tunnelConn = tunRes.conn
			tunnelStream = tunRes.stream
		}
	}

	return targetConn, tunnelConn, tunnelStream, true
}

// cleanupPendingConnections 清理待处理的连接
func cleanupPendingConnections[T io.Closer](targetCh <-chan targetResultHandler[T], tunnelCh <-chan tunnelResult) {
	select {
	case tr := <-targetCh:
		closeIfNotNil(tr.conn)
	default:
	}
	select {
	case tunRes := <-tunnelCh:
		if tunRes.conn != nil {
			tunRes.conn.Close()
		}
	default:
	}
}

// cleanupTunnelConnection 清理隧道连接
func cleanupTunnelConnection(tunnelCh <-chan tunnelResult) {
	select {
	case tunRes := <-tunnelCh:
		if tunRes.conn != nil {
			tunRes.conn.Close()
		}
	case <-time.After(5 * time.Second):
	}
}

// cleanupTargetConnection 清理目标连接
func cleanupTargetConnection[T io.Closer](targetCh <-chan targetResultHandler[T]) {
	select {
	case tgtRes := <-targetCh:
		closeIfNotNil(tgtRes.conn)
	case <-time.After(5 * time.Second):
	}
}

// closeIfNotNil 安全关闭连接（处理泛型类型的 nil 检查）
func closeIfNotNil[T io.Closer](conn T) {
	// 使用类型断言检查是否为 nil 接口值
	if any(conn) != nil {
		conn.Close()
	}
}

// waitForTCPConnections 等待TCP目标和隧道连接完成（向后兼容包装器）
func waitForTCPConnections(
	tunnelCtx context.Context,
	targetCh <-chan tcpTargetResult,
	tunnelCh <-chan tunnelResult,
	logPrefix, tunnelID, targetAddr string,
) (net.Conn, net.Conn, stream.PackageStreamer, bool) {
	// 转换 channel 类型
	genericCh := make(chan targetResultHandler[net.Conn], 1)
	go func() {
		result := <-targetCh
		genericCh <- targetResultHandler[net.Conn]{conn: result.conn, err: result.err}
	}()
	return waitForConnectionsGeneric(tunnelCtx, genericCh, tunnelCh, logPrefix, tunnelID, targetAddr)
}

// waitForUDPConnections 等待UDP目标和隧道连接完成（向后兼容包装器）
func waitForUDPConnections(
	tunnelCtx context.Context,
	targetCh <-chan udpTargetResult,
	tunnelCh <-chan tunnelResult,
	logPrefix, tunnelID, targetAddr string,
) (*net.UDPConn, net.Conn, stream.PackageStreamer, bool) {
	// 转换 channel 类型
	genericCh := make(chan targetResultHandler[*net.UDPConn], 1)
	go func() {
		result := <-targetCh
		genericCh <- targetResultHandler[*net.UDPConn]{conn: result.conn, err: result.err}
	}()
	return waitForConnectionsGeneric(tunnelCtx, genericCh, tunnelCh, logPrefix, tunnelID, targetAddr)
}

// getTunnelReaderWriter 从隧道流中获取 Reader 和 Writer
// 如果流的方法返回 nil，则尝试从连接获取
func getTunnelReaderWriter(tunnelStream stream.PackageStreamer, tunnelConn net.Conn, logPrefix, tunnelID string) (io.Reader, io.Writer, bool) {
	tunnelReader := tunnelStream.GetReader()
	tunnelWriter := tunnelStream.GetWriter()

	// 如果 GetReader 返回 nil，尝试使用 tunnelConn
	if tunnelReader == nil {
		if tunnelConn != nil {
			if reader, ok := tunnelConn.(io.Reader); ok && reader != nil {
				tunnelReader = reader
			} else {
				corelog.Errorf("%s[%s]: tunnelConn does not implement io.Reader or reader is nil", logPrefix, tunnelID)
				return nil, nil, false
			}
		} else {
			corelog.Errorf("%s[%s]: tunnelConn is nil and GetReader() returned nil", logPrefix, tunnelID)
			return nil, nil, false
		}
	}

	// 如果 GetWriter 返回 nil，尝试使用 tunnelConn
	if tunnelWriter == nil {
		if tunnelConn != nil {
			if writer, ok := tunnelConn.(io.Writer); ok && writer != nil {
				tunnelWriter = writer
			} else {
				corelog.Errorf("%s[%s]: tunnelConn does not implement io.Writer or writer is nil", logPrefix, tunnelID)
				return nil, nil, false
			}
		} else {
			corelog.Errorf("%s[%s]: tunnelConn is nil and GetWriter() returned nil", logPrefix, tunnelID)
			return nil, nil, false
		}
	}

	return tunnelReader, tunnelWriter, true
}

// createTunnelRWC 创建隧道的 ReadWriteCloser
func createTunnelRWC(tunnelReader io.Reader, tunnelWriter io.Writer, tunnelStream stream.PackageStreamer, tunnelConn net.Conn, logPrefix, tunnelID string) (io.ReadWriteCloser, bool) {
	if tunnelReader == nil || tunnelWriter == nil {
		corelog.Errorf("%s[%s]: tunnelReader or tunnelWriter is nil after setup", logPrefix, tunnelID)
		return nil, false
	}

	tunnelRWC, err := utils.NewReadWriteCloser(tunnelReader, tunnelWriter, func() error {
		tunnelStream.Close()
		if tunnelConn != nil {
			tunnelConn.Close()
		}
		return nil
	})
	if err != nil {
		corelog.Errorf("%s[%s]: failed to create tunnel ReadWriteCloser: %v", logPrefix, tunnelID, err)
		return nil, false
	}

	return tunnelRWC, true
}

// startCancellationWatcher 启动取消信号监听 goroutine
func startCancellationWatcher(tunnelCtx context.Context, logPrefix, tunnelID string, connections ...io.Closer) {
	go func() {
		<-tunnelCtx.Done()
		corelog.Warnf("%s[%s]: received close notification (context canceled), closing connections", logPrefix, tunnelID)
		for _, conn := range connections {
			if conn != nil {
				if err := conn.Close(); err != nil {
					corelog.Debugf("%s[%s]: connection close returned: %v", logPrefix, tunnelID, err)
				}
			}
		}
	}()
}

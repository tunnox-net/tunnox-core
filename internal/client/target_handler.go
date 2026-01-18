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
	"tunnox-core/internal/utils/iocopy"
)

// handleTunnelOpenRequest 处理隧道打开请求（作为目标客户端）
func (c *TunnoxClient) handleTunnelOpenRequest(cmdBody string) {
	var req struct {
		TunnelID      string `json:"tunnel_id"`
		MappingID     string `json:"mapping_id"`
		SecretKey     string `json:"secret_key"`
		TargetHost    string `json:"target_host"`
		TargetPort    int    `json:"target_port"`
		Protocol      string `json:"protocol"`       // tcp/udp/socks5
		TargetNetwork string `json:"target_network"` // tcp/udp（传输层，用于 SOCKS5 UDP）

		EnableCompression bool   `json:"enable_compression"`
		CompressionLevel  int    `json:"compression_level"`
		EnableEncryption  bool   `json:"enable_encryption"`
		EncryptionMethod  string `json:"encryption_method"`
		EncryptionKey     string `json:"encryption_key"`
		BandwidthLimit    int64  `json:"bandwidth_limit"`
	}

	if err := json.Unmarshal([]byte(cmdBody), &req); err != nil {
		corelog.Errorf("Client: failed to parse tunnel open request: %v", err)
		return
	}

	transformConfig := &transform.TransformConfig{
		BandwidthLimit: req.BandwidthLimit,
	}

	protocol := req.Protocol
	if protocol == "" {
		protocol = "tcp"
	}

	// 如果指定了 TargetNetwork=udp，强制使用 UDP 处理器
	if req.TargetNetwork == "udp" {
		go c.handleUDPTargetTunnel(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
		return
	}

	switch protocol {
	case "tcp":
		go c.handleTCPTargetTunnel(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
	case "udp":
		go c.handleUDPTargetTunnel(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
	case "socks5", "socks":
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
	result := iocopy.Bidirectional(targetConn, tunnelRWC, &iocopy.Options{
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

	// DNS 请求特殊处理：所有 UDP 53 端口请求都使用本地 DNS 解析
	// 这样可以让 CDN 根据服务器 IP 返回最优节点
	if targetPort == 53 {
		go c.handleLocalDNSProxy(tunnelID, mappingID, secretKey, targetHost, logPrefix)
		return
	}

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
	iocopy.UDP(targetConn, tunnelRWC, &iocopy.Options{
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

	tunnelRWC, err := iocopy.NewReadWriteCloser(tunnelReader, tunnelWriter, func() error {
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

// handleLocalDNSProxy 本地 DNS 代理
// 接收 DNS 查询，使用本地系统 DNS 解析，返回结果
// 这样可以让 CDN 根据服务器 IP 返回最优节点
func (c *TunnoxClient) handleLocalDNSProxy(tunnelID, mappingID, secretKey, originalDNS, logPrefix string) {
	corelog.Infof("%s[%s]: using local DNS proxy instead of forwarding to %s", logPrefix, tunnelID, originalDNS)

	// 注册隧道到管理器
	tunnelCtx, tunnelCancel := c.targetTunnelManager.RegisterTunnel(tunnelID, c.Ctx())
	defer func() {
		tunnelCancel()
		c.targetTunnelManager.UnregisterTunnel(tunnelID)
	}()

	// 建立隧道连接
	tunnelConn, tunnelStream, err := c.dialTunnel(tunnelID, mappingID, secretKey)
	if err != nil {
		corelog.Errorf("%s[%s]: failed to dial tunnel: %v", logPrefix, tunnelID, err)
		return
	}
	defer tunnelConn.Close()

	// 获取 Reader/Writer
	tunnelReader := tunnelStream.GetReader()
	tunnelWriter := tunnelStream.GetWriter()
	if tunnelReader == nil || tunnelWriter == nil {
		corelog.Errorf("%s[%s]: tunnel reader or writer is nil", logPrefix, tunnelID)
		return
	}

	// 监听取消信号
	go func() {
		<-tunnelCtx.Done()
		tunnelConn.Close()
	}()

	// 读取 DNS 查询包
	// 注意：UDP 数据包在隧道中有 2 字节长度前缀（大端序）
	buf := make([]byte, 65536)
	for {
		select {
		case <-tunnelCtx.Done():
			return
		default:
		}

		// 读取长度前缀（2字节，大端序）
		_, err := io.ReadFull(tunnelReader, buf[:2])
		if err != nil {
			if err != io.EOF {
				corelog.Debugf("%s[%s]: read length prefix error: %v", logPrefix, tunnelID, err)
			}
			return
		}

		packetLen := int(buf[0])<<8 | int(buf[1])
		if packetLen < 12 || packetLen > 65535 {
			corelog.Warnf("%s[%s]: invalid DNS packet length: %d", logPrefix, tunnelID, packetLen)
			continue
		}

		// 读取实际的 DNS 数据
		_, err = io.ReadFull(tunnelReader, buf[:packetLen])
		if err != nil {
			if err != io.EOF {
				corelog.Debugf("%s[%s]: read DNS packet error: %v", logPrefix, tunnelID, err)
			}
			return
		}

		dnsQuery := buf[:packetLen]

		// 解析 DNS 查询并用本地 DNS 解析
		response, err := c.resolveWithLocalDNS(dnsQuery, logPrefix, tunnelID)
		if err != nil {
			corelog.Warnf("%s[%s]: local DNS resolve failed: %v, forwarding to %s", logPrefix, tunnelID, err, originalDNS)
			// 失败时回退到转发
			response, err = c.forwardDNSQuery(dnsQuery, originalDNS)
			if err != nil {
				corelog.Errorf("%s[%s]: DNS forward also failed: %v", logPrefix, tunnelID, err)
				continue
			}
		}

		// 写回响应（加上 2 字节长度前缀）
		respLen := len(response)
		respBuf := make([]byte, 2+respLen)
		respBuf[0] = byte(respLen >> 8)
		respBuf[1] = byte(respLen)
		copy(respBuf[2:], response)

		if _, err := tunnelWriter.Write(respBuf); err != nil {
			corelog.Debugf("%s[%s]: write response error: %v", logPrefix, tunnelID, err)
			return
		}
	}
}

// resolveWithLocalDNS 使用本地系统 DNS 解析
func (c *TunnoxClient) resolveWithLocalDNS(query []byte, logPrefix, tunnelID string) ([]byte, error) {
	if len(query) < 12 {
		return nil, fmt.Errorf("invalid DNS query: too short")
	}

	// 解析 DNS 查询包
	domain, qtype, err := parseDNSQuery(query)
	if err != nil {
		return nil, err
	}

	corelog.Infof("%s[%s]: local DNS lookup: %s (type=%d)", logPrefix, tunnelID, domain, qtype)

	// 使用系统 DNS 解析
	var ips []net.IP
	if qtype == 1 { // A 记录
		addrs, err := net.LookupIP(domain)
		if err != nil {
			return nil, fmt.Errorf("lookup failed: %v", err)
		}
		for _, addr := range addrs {
			if ipv4 := addr.To4(); ipv4 != nil {
				ips = append(ips, ipv4)
			}
		}
	} else if qtype == 28 { // AAAA 记录
		addrs, err := net.LookupIP(domain)
		if err != nil {
			return nil, fmt.Errorf("lookup failed: %v", err)
		}
		for _, addr := range addrs {
			if addr.To4() == nil {
				ips = append(ips, addr)
			}
		}
	} else {
		return nil, fmt.Errorf("unsupported query type: %d", qtype)
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no records found for %s", domain)
	}

	corelog.Infof("%s[%s]: local DNS resolved %s -> %v", logPrefix, tunnelID, domain, ips)

	// 构造 DNS 响应
	return buildDNSResponse(query, ips, qtype)
}

// forwardDNSQuery 转发 DNS 查询到指定服务器
func (c *TunnoxClient) forwardDNSQuery(query []byte, dnsServer string) ([]byte, error) {
	conn, err := net.DialTimeout("udp", dnsServer+":53", 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := conn.Write(query); err != nil {
		return nil, err
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

// parseDNSQuery 解析 DNS 查询包，提取域名和查询类型
func parseDNSQuery(query []byte) (string, uint16, error) {
	if len(query) < 12 {
		return "", 0, fmt.Errorf("query too short")
	}

	// 跳过 DNS 头部 (12 字节)
	offset := 12
	var domain string

	// 解析域名
	for offset < len(query) {
		length := int(query[offset])
		if length == 0 {
			offset++
			break
		}
		if offset+1+length > len(query) {
			return "", 0, fmt.Errorf("invalid domain name")
		}
		if domain != "" {
			domain += "."
		}
		domain += string(query[offset+1 : offset+1+length])
		offset += 1 + length
	}

	// 读取查询类型
	if offset+2 > len(query) {
		return "", 0, fmt.Errorf("missing query type")
	}
	qtype := uint16(query[offset])<<8 | uint16(query[offset+1])

	return domain, qtype, nil
}

// buildDNSResponse 构造 DNS 响应包
func buildDNSResponse(query []byte, ips []net.IP, qtype uint16) ([]byte, error) {
	if len(query) < 12 {
		return nil, fmt.Errorf("invalid query")
	}

	// 复制查询头部作为响应头部
	response := make([]byte, 0, 512)
	response = append(response, query[:2]...) // Transaction ID

	// 设置标志位: QR=1 (响应), AA=0, TC=0, RD=1, RA=1, RCODE=0
	response = append(response, 0x81, 0x80)

	// QDCOUNT = 1
	response = append(response, 0x00, 0x01)

	// ANCOUNT = len(ips)
	response = append(response, byte(len(ips)>>8), byte(len(ips)))

	// NSCOUNT = 0, ARCOUNT = 0
	response = append(response, 0x00, 0x00, 0x00, 0x00)

	// 复制问题部分
	questionEnd := 12
	for questionEnd < len(query) {
		if query[questionEnd] == 0 {
			questionEnd += 5 // null + QTYPE(2) + QCLASS(2)
			break
		}
		questionEnd += int(query[questionEnd]) + 1
	}
	response = append(response, query[12:questionEnd]...)

	// 添加答案部分
	for _, ip := range ips {
		// 名称指针 (指向问题部分的域名)
		response = append(response, 0xc0, 0x0c)

		if qtype == 1 && ip.To4() != nil { // A 记录
			response = append(response, 0x00, 0x01)             // TYPE = A
			response = append(response, 0x00, 0x01)             // CLASS = IN
			response = append(response, 0x00, 0x00, 0x01, 0x2c) // TTL = 300
			response = append(response, 0x00, 0x04)             // RDLENGTH = 4
			response = append(response, ip.To4()...)
		} else if qtype == 28 && ip.To4() == nil { // AAAA 记录
			response = append(response, 0x00, 0x1c)             // TYPE = AAAA
			response = append(response, 0x00, 0x01)             // CLASS = IN
			response = append(response, 0x00, 0x00, 0x01, 0x2c) // TTL = 300
			response = append(response, 0x00, 0x10)             // RDLENGTH = 16
			response = append(response, ip.To16()...)
		}
	}

	return response, nil
}

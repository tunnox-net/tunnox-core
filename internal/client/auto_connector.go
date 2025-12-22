package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/stream"

	"github.com/google/uuid"
)

// ServerEndpoint 服务器端点定义
type ServerEndpoint struct {
	Protocol string // tcp, udp, quic, websocket
	Address  string // 完整地址
}

// DefaultServerEndpoints 默认服务器端点列表（按优先级排序）
// 使用常量文件中定义的公共服务端点
var DefaultServerEndpoints = []ServerEndpoint{
	{Protocol: "quic", Address: PublicServiceQUIC1},           // quic://tunnox.mydtc.net:8443
	{Protocol: "quic", Address: PublicServiceQUIC2},           // quic://gw.tunnox.net:8443
	{Protocol: "tcp", Address: PublicServiceTCP1},             // tcp://tunnox.mydtc.net:8080
	{Protocol: "tcp", Address: PublicServiceTCP2},             // tcp://gw.tunnox.net:8080
	{Protocol: "websocket", Address: PublicServiceWebSocket1}, // ws://tunnox.mydtc.net
	{Protocol: "websocket", Address: PublicServiceWebSocket2}, // wss://ws.tunnox.net
	{Protocol: "kcp", Address: PublicServiceKCP},
	{Protocol: "httppoll", Address: PublicServiceHTTPPoll},
}

// ConnectionAttempt 连接尝试结果
type ConnectionAttempt struct {
	Endpoint ServerEndpoint
	Conn     net.Conn
	Stream   stream.PackageStreamer
	Err      error
	Index    int // 端点索引（用于优先级判断，索引越小优先级越高）
}

// AutoConnector 自动连接器，负责多协议并发连接尝试
type AutoConnector struct {
	*dispose.ServiceBase
	client *TunnoxClient
}

// sendHandshakeWithContext 在指定的stream上发送握手请求（带context超时控制）
func (ac *AutoConnector) sendHandshakeWithContext(ctx context.Context, stream stream.PackageStreamer, connectionType string) error {
	// 创建一个channel来接收握手结果
	resultChan := make(chan error, 1)

	go func() {
		err := ac.client.sendHandshakeOnStream(stream, connectionType)
		select {
		case resultChan <- err:
		case <-ctx.Done():
			// Context已取消，不发送结果
		}
	}()

	select {
	case err := <-resultChan:
		return err
	case <-ctx.Done():
		return fmt.Errorf("handshake timeout: %w", ctx.Err())
	}
}

// NewAutoConnector 创建自动连接器
func NewAutoConnector(ctx context.Context, client *TunnoxClient) *AutoConnector {
	ac := &AutoConnector{
		ServiceBase: dispose.NewService("AutoConnector", ctx),
		client:      client,
	}

	ac.AddCleanHandler(func() error {
		return nil
	})

	return ac
}

// ConnectWithAutoDetection 自动检测并连接，返回第一个成功的连接尝试（包含已建立的连接）
// 实现多轮重试机制：
// - 第1轮：每个协议5秒超时
// - 第2轮：每个协议10秒超时
// - 第3轮：每个协议15秒超时
func (ac *AutoConnector) ConnectWithAutoDetection(ctx context.Context) (*ConnectionAttempt, error) {
	// 定义每轮的超时时间
	roundTimeouts := []time.Duration{
		time.Duration(AutoConnectRound1Timeout) * time.Second,
		time.Duration(AutoConnectRound2Timeout) * time.Second,
		time.Duration(AutoConnectRound3Timeout) * time.Second,
	}

	// 尝试多轮连接
	for round := 0; round < len(roundTimeouts); round++ {
		timeout := roundTimeouts[round]

		// 显示当前轮次信息
		if round == 0 {
			fmt.Fprintf(os.Stderr, "   Trying endpoints: quic(mydtc/gw), tcp(mydtc/gw), ws(mydtc/ws.tunnox), kcp, httppoll\n")
		} else {
			fmt.Fprintf(os.Stderr, "   Retrying (round %d/%d, timeout: %ds)...\n", round+1, len(roundTimeouts), int(timeout.Seconds()))
		}

		// 尝试当前轮次
		attempt, err := ac.tryRound(ctx, timeout, round+1)
		if err == nil && attempt != nil {
			return attempt, nil
		}

		// 如果context被取消，立即返回
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	// 所有轮次都失败
	return nil, fmt.Errorf("all connection attempts failed after %d rounds", len(roundTimeouts))
}

// tryRound 尝试一轮连接（并发尝试所有协议）
func (ac *AutoConnector) tryRound(ctx context.Context, timeout time.Duration, roundNum int) (*ConnectionAttempt, error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultChan := make(chan *ConnectionAttempt, len(DefaultServerEndpoints))
	var wg sync.WaitGroup

	// 为每个端点启动连接尝试
	for i, endpoint := range DefaultServerEndpoints {
		wg.Add(1)
		go func(ep ServerEndpoint, idx int) {
			defer wg.Done()

			attempt := ac.tryConnectWithTimeout(attemptCtx, ep, timeout)
			attempt.Index = idx

			// 发送结果
			select {
			case resultChan <- attempt:
			case <-attemptCtx.Done():
				// Context已取消，清理资源
				ac.closeAttempt(attempt)
			}
		}(endpoint, i)
	}

	// 等待所有连接尝试完成或context取消
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集所有成功的连接
	successfulAttempts := make([]*ConnectionAttempt, 0)

	for attempt := range resultChan {
		if attempt.Err == nil {
			successfulAttempts = append(successfulAttempts, attempt)
		}
	}

	// 如果没有成功的连接，返回错误
	if len(successfulAttempts) == 0 {
		return nil, fmt.Errorf("all protocols failed in round %d", roundNum)
	}

	// 按优先级排序（Index越小优先级越高）
	sort.Slice(successfulAttempts, func(i, j int) bool {
		return successfulAttempts[i].Index < successfulAttempts[j].Index
	})

	// 按优先级顺序尝试握手，直到成功
	var lastHandshakeErr error
	for _, attempt := range successfulAttempts {
		// 设置配置
		originalProtocol := ac.client.config.Server.Protocol
		originalAddress := ac.client.config.Server.Address
		ac.client.config.Server.Protocol = attempt.Endpoint.Protocol
		ac.client.config.Server.Address = attempt.Endpoint.Address

		// 使用固定的握手超时时间（10秒）
		handshakeCtx, handshakeCancel := context.WithTimeout(ctx, 10*time.Second)
		handshakeErr := ac.sendHandshakeWithContext(handshakeCtx, attempt.Stream, "control")
		handshakeCancel()

		ac.client.config.Server.Protocol = originalProtocol
		ac.client.config.Server.Address = originalAddress

		if handshakeErr == nil {
			// 握手成功，显示成功信息
			fmt.Fprintf(os.Stderr, "   Protocol: %s\n", attempt.Endpoint.Protocol)

			// 关闭其他连接
			for _, otherAttempt := range successfulAttempts {
				if otherAttempt != attempt {
					ac.closeAttempt(otherAttempt)
				}
			}

			return attempt, nil
		}

		// 握手失败，记录错误并尝试下一个
		lastHandshakeErr = handshakeErr
		ac.closeAttempt(attempt)
	}

	// 所有握手都失败
	return nil, fmt.Errorf("all handshakes failed in round %d, last error: %w", roundNum, lastHandshakeErr)
}

// tryConnectWithTimeout 尝试连接到指定端点（带超时）
// 连接建立使用超时context，但Stream创建使用父context以避免过早关闭
func (ac *AutoConnector) tryConnectWithTimeout(ctx context.Context, endpoint ServerEndpoint, timeout time.Duration) *ConnectionAttempt {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 使用超时context建立连接，但使用父context创建Stream
	return ac.tryConnectWithStreamContext(timeoutCtx, ctx, endpoint)
}

// tryConnectWithStreamContext 使用connCtx建立连接，使用streamCtx创建Stream
func (ac *AutoConnector) tryConnectWithStreamContext(connCtx, streamCtx context.Context, endpoint ServerEndpoint) *ConnectionAttempt {
	attempt := &ConnectionAttempt{
		Endpoint: endpoint,
	}

	// 检查 context 是否已经被取消
	select {
	case <-connCtx.Done():
		attempt.Err = connCtx.Err()
		return attempt
	default:
	}

	// 根据协议尝试连接
	var conn net.Conn
	var err error

	switch endpoint.Protocol {
	case "tcp":
		// TCP 连接使用 DialContext 以支持 context 取消
		dialer := &net.Dialer{}
		conn, err = dialer.DialContext(connCtx, "tcp", endpoint.Address)
		if err == nil {
			SetKeepAliveIfSupported(conn, true)
		}
	case "websocket":
		conn, err = dialWebSocket(connCtx, endpoint.Address)
	case "quic":
		conn, err = dialQUIC(connCtx, endpoint.Address)
	case "kcp":
		conn, err = dialKCP(connCtx, endpoint.Address)
	case "httppoll", "http-long-polling", "httplp":
		tempInstanceID := uuid.New().String()
		conn, err = dialHTTPLongPolling(connCtx, endpoint.Address, 0, "", tempInstanceID, "")
	default:
		attempt.Err = fmt.Errorf("unsupported protocol: %s", endpoint.Protocol)
		return attempt
	}

	if err != nil {
		attempt.Err = err
		return attempt
	}

	// 检查 context 是否已经被取消
	select {
	case <-connCtx.Done():
		conn.Close()
		attempt.Err = connCtx.Err()
		return attempt
	default:
	}

	// 创建 Stream（使用streamCtx，不受连接超时影响）
	var pkgStream stream.PackageStreamer
	if endpoint.Protocol == "httppoll" || endpoint.Protocol == "http-long-polling" || endpoint.Protocol == "httplp" {
		if httppollConn, ok := conn.(*HTTPLongPollingConn); ok {
			baseURL := httppollConn.baseURL
			// 构建 push/poll URL（与 NewHTTPLongPollingConn 保持一致）
			var pushURL, pollURL string
			if strings.Contains(baseURL, "/_tunnox") {
				pushURL = baseURL + "/push"
				pollURL = baseURL + "/poll"
			} else {
				pushURL = baseURL + "/_tunnox/v1/push"
				pollURL = baseURL + "/_tunnox/v1/poll"
			}
			pkgStream = httppoll.NewStreamProcessor(streamCtx, baseURL, pushURL, pollURL, 0, "", httppollConn.instanceID, "")
			if httppollConn.connectionID != "" {
				pkgStream.(*httppoll.StreamProcessor).SetConnectionID(httppollConn.connectionID)
			}
		} else {
			streamFactory := stream.NewDefaultStreamFactory(streamCtx)
			pkgStream = streamFactory.CreateStreamProcessor(conn, conn)
		}
	} else {
		streamFactory := stream.NewDefaultStreamFactory(streamCtx)
		pkgStream = streamFactory.CreateStreamProcessor(conn, conn)
	}

	attempt.Conn = conn
	attempt.Stream = pkgStream
	return attempt
}

// tryConnect 尝试连接到指定端点
func (ac *AutoConnector) tryConnect(ctx context.Context, endpoint ServerEndpoint) *ConnectionAttempt {
	attempt := &ConnectionAttempt{
		Endpoint: endpoint,
	}

	// 检查 context 是否已经被取消
	select {
	case <-ctx.Done():
		attempt.Err = ctx.Err()
		return attempt
	default:
	}

	// 根据协议尝试连接
	var conn net.Conn
	var err error

	switch endpoint.Protocol {
	case "tcp":
		// TCP 连接使用 DialContext 以支持 context 取消
		dialer := &net.Dialer{}
		conn, err = dialer.DialContext(ctx, "tcp", endpoint.Address)
		if err == nil {
			SetKeepAliveIfSupported(conn, true)
		}
	case "websocket":
		conn, err = dialWebSocket(ctx, endpoint.Address)
	case "quic":
		conn, err = dialQUIC(ctx, endpoint.Address)
	case "kcp":
		conn, err = dialKCP(ctx, endpoint.Address)
	case "httppoll", "http-long-polling", "httplp":
		tempInstanceID := uuid.New().String()
		conn, err = dialHTTPLongPolling(ctx, endpoint.Address, 0, "", tempInstanceID, "")
	default:
		attempt.Err = fmt.Errorf("unsupported protocol: %s", endpoint.Protocol)
		return attempt
	}

	if err != nil {
		attempt.Err = err
		return attempt
	}

	// 检查 context 是否已经被取消
	select {
	case <-ctx.Done():
		conn.Close()
		attempt.Err = ctx.Err()
		return attempt
	default:
	}

	// 创建 Stream
	var pkgStream stream.PackageStreamer
	if endpoint.Protocol == "httppoll" || endpoint.Protocol == "http-long-polling" || endpoint.Protocol == "httplp" {
		if httppollConn, ok := conn.(*HTTPLongPollingConn); ok {
			baseURL := httppollConn.baseURL
			pushURL := baseURL + "/_tunnox/v1/push"
			pollURL := baseURL + "/_tunnox/v1/poll"
			pkgStream = httppoll.NewStreamProcessor(ctx, baseURL, pushURL, pollURL, 0, "", httppollConn.instanceID, "")
			if httppollConn.connectionID != "" {
				pkgStream.(*httppoll.StreamProcessor).SetConnectionID(httppollConn.connectionID)
			}
		} else {
			streamFactory := stream.NewDefaultStreamFactory(ctx)
			pkgStream = streamFactory.CreateStreamProcessor(conn, conn)
		}
	} else {
		streamFactory := stream.NewDefaultStreamFactory(ctx)
		pkgStream = streamFactory.CreateStreamProcessor(conn, conn)
	}

	attempt.Conn = conn
	attempt.Stream = pkgStream
	return attempt
}

// closeAttempt 关闭连接尝试的资源
// closeAttempt 关闭连接尝试的资源
func (ac *AutoConnector) closeAttempt(attempt *ConnectionAttempt) {
	if attempt == nil {
		return
	}

	// 安全关闭Stream（无返回值）
	if attempt.Stream != nil {
		attempt.Stream.Close()
	}

	// 安全关闭Conn（忽略错误，避免panic）
	if attempt.Conn != nil {
		_ = attempt.Conn.Close()
	}
}

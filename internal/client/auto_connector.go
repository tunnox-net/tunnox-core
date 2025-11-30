package client

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/stream"
)

// ServerEndpoint 服务器端点定义
type ServerEndpoint struct {
	Protocol string // tcp, udp, quic, websocket
	Address  string // 完整地址
}

// DefaultServerEndpoints 默认服务器端点列表
var DefaultServerEndpoints = []ServerEndpoint{
	{Protocol: "tcp", Address: "gw.tunnox.net:8000"},
	{Protocol: "udp", Address: "gw.tunnox.net:8000"},
	{Protocol: "quic", Address: "gw.tunnox.net:443"},
	{Protocol: "websocket", Address: "https://gw.tunnox.net/_tunnox"},
	{Protocol: "httppoll", Address: "https://gw.tunnox.net"},
}

// ConnectionAttempt 连接尝试结果
type ConnectionAttempt struct {
	Endpoint ServerEndpoint
	Conn     net.Conn
	Stream   stream.PackageStreamer
	Err      error
}

// AutoConnector 自动连接器，负责多协议并发连接尝试
type AutoConnector struct {
	*dispose.ServiceBase
	client *TunnoxClient
	mu     sync.RWMutex
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

// ConnectWithAutoDetection 自动检测并连接，返回第一个成功的端点
func (ac *AutoConnector) ConnectWithAutoDetection(ctx context.Context) (*ServerEndpoint, error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultChan := make(chan *ConnectionAttempt, len(DefaultServerEndpoints))
	var wg sync.WaitGroup

	// 为每个端点启动连接尝试
	for _, endpoint := range DefaultServerEndpoints {
		wg.Add(1)
		go func(ep ServerEndpoint) {
			defer wg.Done()
			attempt := ac.tryConnect(attemptCtx, ep)
			
			// 必须发送结果，即使 context 被取消也要发送
			// 使用超时机制确保不会永久阻塞
			sendTimeout := time.NewTimer(2 * time.Second)
			defer sendTimeout.Stop()
			
			select {
			case resultChan <- attempt:
				// 成功发送
			case <-attemptCtx.Done():
				// Context 已取消，仍然尝试发送（非阻塞）
				select {
				case resultChan <- attempt:
					// 成功发送
				default:
					// Channel 可能已满或关闭，关闭连接
					ac.closeAttempt(attempt)
				}
			case <-sendTimeout.C:
				// 发送超时，关闭连接（这种情况不应该发生）
				ac.closeAttempt(attempt)
			}
		}(endpoint)
	}

	// 等待所有连接尝试完成
	var firstSuccess *ConnectionAttempt
	var allErrors []error
	receivedCount := 0

	// 使用超时机制防止死锁（15秒，足够所有连接尝试完成）
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()

	for receivedCount < len(DefaultServerEndpoints) {
		select {
		case attempt := <-resultChan:
			receivedCount++
			if attempt.Err == nil {
				// 成功连接
				if firstSuccess == nil {
					firstSuccess = attempt
					cancel() // 取消其他尝试
					// 不立即返回，继续等待其他结果（但会关闭它们）
				} else {
					// 已经有成功连接，关闭这个
					ac.closeAttempt(attempt)
				}
			} else {
				allErrors = append(allErrors, attempt.Err)
			}
		case <-ctx.Done():
			// Context 被取消，等待所有 goroutine 完成
			wg.Wait()
			if firstSuccess != nil {
				return &firstSuccess.Endpoint, nil
			}
			return nil, ctx.Err()
		case <-timeout.C:
			// 超时，等待所有 goroutine 完成
			wg.Wait()
			if firstSuccess != nil {
				return &firstSuccess.Endpoint, nil
			}
			// 如果超时且没有成功连接，返回错误
			return nil, fmt.Errorf("auto connection timeout after 15s (received %d/%d results): %v", 
				receivedCount, len(DefaultServerEndpoints), allErrors)
		}
	}

	// 等待所有 goroutine 完成（确保资源清理）
	wg.Wait()

	if firstSuccess != nil {
		return &firstSuccess.Endpoint, nil
	}

	// 所有连接都失败
	return nil, fmt.Errorf("all connection attempts failed: %v", allErrors)
}

// tryConnect 尝试连接到指定端点
func (ac *AutoConnector) tryConnect(ctx context.Context, endpoint ServerEndpoint) *ConnectionAttempt {
	attempt := &ConnectionAttempt{
		Endpoint: endpoint,
	}

	// 设置超时
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 根据协议尝试连接
	var conn net.Conn
	var err error

	switch endpoint.Protocol {
	case "tcp":
		conn, err = net.DialTimeout("tcp", endpoint.Address, 5*time.Second)
		if err == nil {
			// 配置 TCP 连接选项
			// 使用接口而不是具体类型
			SetKeepAliveIfSupported(conn, true)
		}
	case "udp":
		conn, err = dialUDPControlConnection(endpoint.Address)
	case "websocket":
		conn, err = dialWebSocket(timeoutCtx, endpoint.Address)
	case "quic":
		conn, err = dialQUIC(timeoutCtx, endpoint.Address)
	case "httppoll", "http-long-polling", "httplp":
		// HTTP 长轮询需要 clientID 和 token，自动连接时使用 0 和空字符串
		conn, err = dialHTTPLongPolling(timeoutCtx, endpoint.Address, 0, "")
	default:
		attempt.Err = fmt.Errorf("unsupported protocol: %s", endpoint.Protocol)
		return attempt
	}

	if err != nil {
		attempt.Err = fmt.Errorf("failed to dial %s://%s: %w", endpoint.Protocol, endpoint.Address, err)
		return attempt
	}

	// 创建 Stream
	streamFactory := stream.NewDefaultStreamFactory(timeoutCtx)
	stream := streamFactory.CreateStreamProcessor(conn, conn)

	attempt.Conn = conn
	attempt.Stream = stream
	return attempt
}

// closeAttempt 关闭连接尝试的资源
func (ac *AutoConnector) closeAttempt(attempt *ConnectionAttempt) {
	if attempt.Stream != nil {
		attempt.Stream.Close()
	}
	if attempt.Conn != nil {
		attempt.Conn.Close()
	}
}


package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	coreErrors "tunnox-core/internal/core/errors"
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
// 优先级从高到低：quic > tcp > websocket > httppoll
var DefaultServerEndpoints = []ServerEndpoint{
	{Protocol: "quic", Address: "gw.tunnox.net:443"},
	{Protocol: "tcp", Address: "gw.tunnox.net:8000"},
	{Protocol: "websocket", Address: "https://gw.tunnox.net/_tunnox"},
	{Protocol: "httppoll", Address: "https://gw.tunnox.net"},
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
func (ac *AutoConnector) ConnectWithAutoDetection(ctx context.Context) (*ConnectionAttempt, error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultChan := make(chan *ConnectionAttempt, len(DefaultServerEndpoints))
	var wg sync.WaitGroup

	// 输出自动连接提示
	fmt.Fprintf(os.Stderr, "🔍 Auto-connecting: trying %d endpoints...\n", len(DefaultServerEndpoints))

	// 为每个端点启动连接尝试
	// 高优先级（前3个：quic, tcp, websocket）立即启动
	// 低优先级（httppoll）延迟2秒启动
	highPriorityCount := 3
	for i, endpoint := range DefaultServerEndpoints {
		wg.Add(1)
		go func(ep ServerEndpoint, idx int) {
			defer wg.Done()

			// 低优先级连接延迟2秒启动
			if idx >= highPriorityCount {
				select {
				case <-time.After(2 * time.Second):
					// 延迟后继续
				case <-attemptCtx.Done():
					// Context 已取消，发送失败结果
					attempt := &ConnectionAttempt{
						Endpoint: ep,
						Index:    idx,
						Err:      attemptCtx.Err(),
					}
					// 非阻塞发送
					select {
					case resultChan <- attempt:
					default:
					}
					return
				}
			}

			// 输出连接尝试信息
			fmt.Fprintf(os.Stderr, "🔍 Trying %s://%s... (%d/%d)\n", ep.Protocol, ep.Address, idx+1, len(DefaultServerEndpoints))

			attempt := ac.tryConnect(attemptCtx, ep)
			attempt.Index = idx // 记录端点索引

			// 输出连接结果
			if attempt.Err == nil {
				fmt.Fprintf(os.Stderr, "✅ Connected via %s://%s\n", ep.Protocol, ep.Address)
			} else {
				fmt.Fprintf(os.Stderr, "❌ Failed to connect via %s://%s: %v\n", ep.Protocol, ep.Address, attempt.Err)
			}

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
				fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to send result for %s://%s (channel full or timeout)\n", ep.Protocol, ep.Address)
				ac.closeAttempt(attempt)
			}
		}(endpoint, i)
	}

	// 等待所有连接尝试完成
	// 使用 map 存储成功连接，key 为端点索引（用于优先级判断）
	successAttempts := make(map[int]*ConnectionAttempt)
	var allErrors []error
	receivedCount := 0
	highPriorityResults := make(map[int]*ConnectionAttempt) // 高优先级连接结果

	// 使用超时机制防止死锁（20秒，足够所有连接尝试完成）
	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()

	for receivedCount < len(DefaultServerEndpoints) {
		select {
		case attempt := <-resultChan:
			receivedCount++
			if attempt.Err == nil {
				// 连接建立成功，但需要完成握手并收到 ACK 才算真正成功
				// 临时设置协议和地址，以便握手时使用正确的协议
				originalProtocol := ac.client.config.Server.Protocol
				originalAddress := ac.client.config.Server.Address
				ac.client.config.Server.Protocol = attempt.Endpoint.Protocol
				ac.client.config.Server.Address = attempt.Endpoint.Address

				// 执行握手（等待 ACK）
				handshakeErr := ac.client.sendHandshakeOnStream(attempt.Stream, "control")

				// 恢复原始配置
				ac.client.config.Server.Protocol = originalProtocol
				ac.client.config.Server.Address = originalAddress

				if handshakeErr != nil {
					// 握手失败，关闭连接，标记为失败
					attempt.Err = coreErrors.Wrap(handshakeErr, coreErrors.ErrorTypeProtocol, "handshake failed")
					ac.closeAttempt(attempt)
					allErrors = append(allErrors, attempt.Err)
					fmt.Fprintf(os.Stderr, "❌ Handshake failed via %s://%s: %v\n", attempt.Endpoint.Protocol, attempt.Endpoint.Address, handshakeErr)
				} else {
					// 握手成功，收到 ACK，记录索引（索引越小优先级越高）
					successAttempts[attempt.Index] = attempt
					fmt.Fprintf(os.Stderr, "✅ Handshake successful via %s://%s (received ACK)\n", attempt.Endpoint.Protocol, attempt.Endpoint.Address)
					// 如果已经有成功连接，取消其他尝试并立即返回
					if len(successAttempts) == 1 {
						cancel() // 取消其他尝试
						// 立即返回第一个成功连接（优先级最高的）
						bestAttempt := attempt
						// 在后台等待并清理其他连接
						go func() {
							wg.Wait()
							// 关闭其他可能成功的连接
							for idx, otherAttempt := range successAttempts {
								if idx != bestAttempt.Index && otherAttempt.Err == nil {
									ac.closeAttempt(otherAttempt)
								}
							}
						}()
						return bestAttempt, nil
					}
				}
			} else {
				allErrors = append(allErrors, attempt.Err)
				// 记录高优先级连接的结果（用于日志和调试）
				if attempt.Index < highPriorityCount {
					highPriorityResults[attempt.Index] = attempt
				}
				// 注意：即使所有高优先级连接都失败，也要等待低优先级连接完成
				// 因为低优先级连接（如 UDP）可能能够成功连接
			}
		case <-ctx.Done():
			// Context 被取消，等待所有 goroutine 完成
			wg.Wait()
			// 选择优先级最高的成功连接（索引最小的）
			if len(successAttempts) > 0 {
				bestIdx := len(DefaultServerEndpoints)
				var bestAttempt *ConnectionAttempt
				for idx, attempt := range successAttempts {
					if idx < bestIdx {
						bestIdx = idx
						bestAttempt = attempt
					}
				}
				// 关闭其他成功连接
				for idx, attempt := range successAttempts {
					if idx != bestIdx {
						ac.closeAttempt(attempt)
					}
				}
				return bestAttempt, nil
			}
			return nil, ctx.Err()
		case <-timeout.C:
			// 超时，等待所有 goroutine 完成
			wg.Wait()
			// 选择优先级最高的成功连接（索引最小的）
			if len(successAttempts) > 0 {
				bestIdx := len(DefaultServerEndpoints)
				var bestAttempt *ConnectionAttempt
				for idx, attempt := range successAttempts {
					if idx < bestIdx {
						bestIdx = idx
						bestAttempt = attempt
					}
				}
				// 关闭其他成功连接
				for idx, attempt := range successAttempts {
					if idx != bestIdx {
						ac.closeAttempt(attempt)
					}
				}
				return bestAttempt, nil
			}
			// 如果超时且没有成功连接，返回错误
			return nil, coreErrors.Newf(coreErrors.ErrorTypeTemporary, "auto connection timeout after 20s (received %d/%d results): %v",
				receivedCount, len(DefaultServerEndpoints), allErrors)
		}
	}

	// 等待所有 goroutine 完成（确保资源清理）
	wg.Wait()

	// 选择优先级最高的成功连接（索引最小的）
	if len(successAttempts) > 0 {
		bestIdx := len(DefaultServerEndpoints)
		var bestAttempt *ConnectionAttempt
		for idx, attempt := range successAttempts {
			if idx < bestIdx {
				bestIdx = idx
				bestAttempt = attempt
			}
		}
		// 关闭其他成功连接
		for idx, attempt := range successAttempts {
			if idx != bestIdx {
				ac.closeAttempt(attempt)
			}
		}
		return bestAttempt, nil
	}

	// 所有连接都失败
	return nil, coreErrors.Newf(coreErrors.ErrorTypeNetwork, "all connection attempts failed: %v", allErrors)
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

	// 设置超时（最多20秒）
	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// 根据协议尝试连接
	var conn net.Conn
	var err error

	switch endpoint.Protocol {
	case "tcp":
		// TCP 连接使用 DialContext 以支持 context 取消
		dialer := &net.Dialer{
			Timeout: 20 * time.Second,
		}
		conn, err = dialer.DialContext(timeoutCtx, "tcp", endpoint.Address)
		if err == nil {
			// 配置 TCP 连接选项
			// 使用接口而不是具体类型
			SetKeepAliveIfSupported(conn, true)
		}
	case "websocket":
		conn, err = dialWebSocket(timeoutCtx, endpoint.Address)
	case "quic":
		conn, err = dialQUIC(timeoutCtx, endpoint.Address)
	case "httppoll", "http-long-polling", "httplp":
		// HTTP 长轮询需要 clientID 和 token，自动连接时使用 0 和空字符串
		// 自动连接阶段生成临时 instanceID（后续会被正式连接替换）
		tempInstanceID := uuid.New().String()
		conn, err = dialHTTPLongPolling(timeoutCtx, endpoint.Address, 0, "", tempInstanceID, "")
	default:
		attempt.Err = coreErrors.Newf(coreErrors.ErrorTypePermanent, "unsupported protocol: %s", endpoint.Protocol)
		return attempt
	}

	if err != nil {
		attempt.Err = coreErrors.Wrapf(err, coreErrors.ErrorTypeNetwork, "failed to dial %s://%s", endpoint.Protocol, endpoint.Address)
		return attempt
	}

	// 检查 context 是否已经被取消（在连接建立后立即检查）
	select {
	case <-ctx.Done():
		// Context 被取消，关闭连接并返回错误
		conn.Close()
		attempt.Err = ctx.Err()
		return attempt
	default:
	}

	// 创建 Stream（使用原始 context，避免超时问题）
	// HTTP Long Polling 需要特殊的 StreamProcessor
	defer func() {
		if r := recover(); r != nil {
			attempt.Err = coreErrors.Newf(coreErrors.ErrorTypeFatal, "panic while creating stream: %v", r)
			if conn != nil {
				conn.Close()
			}
		}
	}()

	var pkgStream stream.PackageStreamer
	if endpoint.Protocol == "httppoll" || endpoint.Protocol == "http-long-polling" || endpoint.Protocol == "httplp" {
		// HTTP Long Polling 需要特殊的 StreamProcessor
		if httppollConn, ok := conn.(*HTTPLongPollingConn); ok {
			baseURL := httppollConn.baseURL
			pushURL := baseURL + "/tunnox/v1/push"
			pollURL := baseURL + "/tunnox/v1/poll"
			// 自动连接时使用 clientID=0 和空 token
			pkgStream = httppoll.NewStreamProcessor(ctx, baseURL, pushURL, pollURL, 0, "", httppollConn.instanceID, "")
			// 设置 ConnectionID
			if httppollConn.connectionID != "" {
				pkgStream.(*httppoll.StreamProcessor).SetConnectionID(httppollConn.connectionID)
			}
		} else {
			// 回退到默认方式
			streamFactory := stream.NewDefaultStreamFactory(ctx)
			pkgStream = streamFactory.CreateStreamProcessor(conn, conn)
		}
	} else {
		// 其他协议使用默认 StreamProcessor
		streamFactory := stream.NewDefaultStreamFactory(ctx)
		pkgStream = streamFactory.CreateStreamProcessor(conn, conn)
	}

	attempt.Conn = conn
	attempt.Stream = pkgStream
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

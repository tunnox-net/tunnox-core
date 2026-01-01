package client

import (
	"context"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/client/transport"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"
)

// ServerEndpoint 服务器端点定义
type ServerEndpoint struct {
	Protocol string // tcp, udp, quic, websocket
	Address  string // 完整地址
}

// DefaultServerEndpoints 返回默认服务器端点列表（按优先级排序，仅包含已编译的协议）
// WebSocket 优先因为穿透性最好
func DefaultServerEndpoints() []ServerEndpoint {
	// 定义所有可能的端点（按优先级排序）
	allEndpoints := []ServerEndpoint{
		{Protocol: "websocket", Address: PublicServiceWebSocket}, // wss://ws.tunnox.net
		{Protocol: "quic", Address: PublicServiceQUIC},           // gw.tunnox.net:8443
		{Protocol: "tcp", Address: PublicServiceTCP},             // gw.tunnox.net:8080
		{Protocol: "kcp", Address: PublicServiceKCP},             // gw.tunnox.net:8000
	}

	// 过滤出已编译的协议
	var available []ServerEndpoint
	for _, ep := range allEndpoints {
		if transport.IsProtocolAvailable(ep.Protocol) {
			available = append(available, ep)
		}
	}
	return available
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

// ConnectWithAutoDetection 自动检测并连接，返回第一个成功的连接尝试（包含已建立的连接和完成的握手）
// 策略：并发尝试所有协议，第一个成功的立即返回
func (ac *AutoConnector) ConnectWithAutoDetection(ctx context.Context) (*ConnectionAttempt, error) {
	// 定义每轮的超时时间
	roundTimeouts := []time.Duration{
		time.Duration(AutoConnectRound1Timeout) * time.Second,
		time.Duration(AutoConnectRound2Timeout) * time.Second,
	}

	// 尝试多轮连接
	for round := 0; round < len(roundTimeouts); round++ {
		timeout := roundTimeouts[round]

		// 显示当前轮次信息
		endpoints := DefaultServerEndpoints()
		if len(endpoints) == 0 {
			return nil, coreerrors.New(coreerrors.CodeNotConfigured, "no protocols available (none compiled in)")
		}
		if round == 0 {
			protocols := make([]string, len(endpoints))
			for i, ep := range endpoints {
				protocols[i] = ep.Protocol
			}
			corelog.Debugf("AutoConnector: trying protocols: %v", protocols)
		} else {
			corelog.Debugf("AutoConnector: retrying (round %d/%d, timeout: %ds)", round+1, len(roundTimeouts), int(timeout.Seconds()))
		}

		// 尝试当前轮次 - 使用快速返回策略
		attempt, err := ac.tryRoundFastReturn(ctx, timeout, round+1)
		if err == nil && attempt != nil {
			return attempt, nil
		}

		// 如果context被取消，立即返回
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		corelog.Debugf("AutoConnector: round %d failed: %v", round+1, err)
	}

	// 所有轮次都失败
	return nil, coreerrors.Newf(coreerrors.CodeConnectionError, "all connection attempts failed after %d rounds", len(roundTimeouts))
}

// tryRoundFastReturn 尝试一轮连接（并发尝试所有协议，第一个成功就返回）
func (ac *AutoConnector) tryRoundFastReturn(ctx context.Context, timeout time.Duration, roundNum int) (*ConnectionAttempt, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 使用channel接收第一个成功的连接
	successChan := make(chan *ConnectionAttempt, 1)
	// 用于收集所有需要清理的连接
	var cleanupMu sync.Mutex
	var toCleanup []*ConnectionAttempt

	var wg sync.WaitGroup

	// 为每个端点启动连接尝试（连接+握手一起）
	endpoints := DefaultServerEndpoints()
	for i, endpoint := range endpoints {
		wg.Add(1)
		go func(ep ServerEndpoint, idx int) {
			defer wg.Done()

			// 尝试连接并握手
			attempt := ac.tryConnectAndHandshake(attemptCtx, ep)
			attempt.Index = idx

			if attempt.Err == nil {
				// 连接成功，尝试发送到成功channel
				select {
				case successChan <- attempt:
					// 成功发送，取消其他尝试
					cancel()
				default:
					// 已经有成功的连接了，需要清理这个
					cleanupMu.Lock()
					toCleanup = append(toCleanup, attempt)
					cleanupMu.Unlock()
				}
			}
		}(endpoint, i)
	}

	// 等待第一个成功或所有尝试完成
	go func() {
		wg.Wait()
		close(successChan)
	}()

	// 等待结果
	select {
	case attempt, ok := <-successChan:
		if ok && attempt != nil {
			corelog.Infof("AutoConnector: connected via %s://%s", attempt.Endpoint.Protocol, attempt.Endpoint.Address)

			// 异步清理其他成功的连接
			go func() {
				wg.Wait() // 等待所有goroutine完成
				cleanupMu.Lock()
				defer cleanupMu.Unlock()
				for _, a := range toCleanup {
					ac.closeAttempt(a)
				}
			}()

			return attempt, nil
		}
	case <-attemptCtx.Done():
		// 超时或取消
	}

	return nil, coreerrors.Newf(coreerrors.CodeConnectionError, "all protocols failed in round %d", roundNum)
}

// tryConnectAndHandshake 尝试连接到指定端点并完成握手
func (ac *AutoConnector) tryConnectAndHandshake(ctx context.Context, endpoint ServerEndpoint) *ConnectionAttempt {
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

	// 使用较短的连接超时
	dialTimeout := time.Duration(AutoConnectDialTimeout) * time.Second
	dialCtx, dialCancel := context.WithTimeout(ctx, dialTimeout)
	defer dialCancel()

	// 使用统一的协议注册表拨号
	conn, err := transport.Dial(dialCtx, endpoint.Protocol, endpoint.Address)
	if err != nil {
		attempt.Err = coreerrors.Wrapf(err, coreerrors.CodeConnectionError, "dial %s failed", endpoint.Protocol)
		return attempt
	}

	// TCP 特殊处理：设置 KeepAlive
	if endpoint.Protocol == "tcp" {
		SetKeepAliveIfSupported(conn, true)
	}

	// 检查 context 是否已经被取消
	select {
	case <-ctx.Done():
		conn.Close()
		attempt.Err = ctx.Err()
		return attempt
	default:
	}

	// 创建 Stream（使用父context，不受连接超时影响）
	streamFactory := stream.NewDefaultStreamFactory(ctx)
	pkgStream := streamFactory.CreateStreamProcessor(conn, conn)

	// 设置握手超时
	handshakeTimeout := time.Duration(AutoConnectHandshakeTimeout) * time.Second
	handshakeCtx, handshakeCancel := context.WithTimeout(ctx, handshakeTimeout)
	defer handshakeCancel()

	// 发送握手（直接传入 protocol，避免并发修改全局配置）
	handshakeErr := ac.sendHandshakeWithContext(handshakeCtx, pkgStream, "control", endpoint.Protocol)

	if handshakeErr != nil {
		pkgStream.Close()
		conn.Close()
		attempt.Err = coreerrors.Wrap(handshakeErr, coreerrors.CodeHandshakeFailed, "handshake failed")
		return attempt
	}

	// 连接和握手都成功
	attempt.Conn = conn
	attempt.Stream = pkgStream
	return attempt
}

// sendHandshakeWithContext 在指定的stream上发送握手请求（带context超时控制）
// protocol: 使用的传输协议，直接传入避免并发修改全局配置
func (ac *AutoConnector) sendHandshakeWithContext(ctx context.Context, stream stream.PackageStreamer, connectionType string, protocol string) error {
	// 创建一个channel来接收握手结果
	resultChan := make(chan error, 1)

	go func() {
		err := ac.client.sendHandshakeOnStream(stream, connectionType, protocol)
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
		return coreerrors.Wrap(ctx.Err(), coreerrors.CodeTimeout, "handshake timeout")
	}
}

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

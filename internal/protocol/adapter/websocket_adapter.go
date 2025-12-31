// Package adapter WebSocket 协议适配器
// 提供 WebSocket 协议的服务端监听和客户端连接能力
// 适用于防火墙穿透、企业网络环境
package adapter

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"tunnox-core/internal/cloud/constants"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/protocol/session"
)

// WebSocket 配置常量
const (
	// WebSocketHandshakeTimeout 握手超时时间
	WebSocketHandshakeTimeout = 20 * time.Second

	// WebSocketDefaultPath 默认 WebSocket 路径
	WebSocketDefaultPath = "/_tunnox"

	// WebSocketPingInterval ping 间隔
	WebSocketPingInterval = 30 * time.Second

	// WebSocketPongTimeout pong 超时
	WebSocketPongTimeout = 60 * time.Second

	// WebSocketWriteTimeout 写超时
	WebSocketWriteTimeout = 10 * time.Second

	// WebSocketCloseTimeout 关闭超时
	WebSocketCloseTimeout = 5 * time.Second
)

// WebSocketAdapter WebSocket 协议适配器
type WebSocketAdapter struct {
	BaseAdapter
	httpServer  *http.Server
	listener    net.Listener
	upgrader    websocket.Upgrader
	connChan    chan *wsServerConn
	closeChan   chan struct{}
	closeOnce   sync.Once
	acceptMutex sync.Mutex
}

// NewWebSocketAdapter 创建 WebSocket 适配器
func NewWebSocketAdapter(parentCtx context.Context, sess session.Session) *WebSocketAdapter {
	w := &WebSocketAdapter{
		BaseAdapter: BaseAdapter{},
		connChan:    make(chan *wsServerConn, 128),
		closeChan:   make(chan struct{}),
		upgrader: websocket.Upgrader{
			HandshakeTimeout:  WebSocketHandshakeTimeout,
			ReadBufferSize:    constants.WebSocketBufferSize,
			WriteBufferSize:   constants.WebSocketBufferSize,
			EnableCompression: false, // 禁用 WebSocket 压缩，Tunnox 有自己的压缩层
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源
			},
		},
	}
	w.SetName("websocket")
	w.SetSession(sess)
	w.SetCtx(parentCtx, w.onClose)
	w.SetProtocolAdapter(w)
	return w
}

// Dial 建立 WebSocket 连接（客户端）
func (w *WebSocketAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	corelog.Infof("WebSocketAdapter: dialing %s", addr)

	// 规范化 URL
	wsURL := normalizeWSURL(addr)

	dialer := websocket.Dialer{
		HandshakeTimeout: WebSocketHandshakeTimeout,
		ReadBufferSize:   constants.WebSocketBufferSize,
		WriteBufferSize:  constants.WebSocketBufferSize,
	}

	conn, _, err := dialer.DialContext(w.Ctx(), wsURL, nil)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError,
			"failed to dial WebSocket")
	}

	corelog.Infof("WebSocketAdapter: connected to %s", addr)

	return newWSClientConn(conn), nil
}

// Listen 启动 WebSocket 监听（服务端）
func (w *WebSocketAdapter) Listen(addr string) error {
	corelog.Infof("WebSocketAdapter: listening on %s", addr)

	// 创建 TCP 监听器
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError,
			"failed to listen TCP for WebSocket")
	}
	w.listener = listener

	// 创建 HTTP 处理器
	mux := http.NewServeMux()
	mux.HandleFunc(WebSocketDefaultPath, w.handleWebSocket)
	mux.HandleFunc("/", w.handleWebSocket) // 也接受根路径

	// 创建 HTTP 服务器
	w.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: WebSocketHandshakeTimeout,
	}

	// 启动 HTTP 服务
	go func() {
		if err := w.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			corelog.Errorf("WebSocketAdapter: HTTP server error: %v", err)
		}
	}()

	corelog.Infof("WebSocketAdapter: listening started on %s", addr)
	return nil
}

// handleWebSocket 处理 WebSocket 升级请求
func (w *WebSocketAdapter) handleWebSocket(wr http.ResponseWriter, r *http.Request) {
	// 检查是否已关闭
	select {
	case <-w.closeChan:
		http.Error(wr, "server closed", http.StatusServiceUnavailable)
		return
	default:
	}

	// 升级为 WebSocket 连接
	conn, err := w.upgrader.Upgrade(wr, r, nil)
	if err != nil {
		corelog.Errorf("WebSocketAdapter: upgrade failed: %v", err)
		return
	}

	corelog.Infof("WebSocketAdapter: accepted connection from %s", r.RemoteAddr)

	// 创建包装连接
	wsConn := newWSServerConn(conn, r.RemoteAddr)

	// 发送到接受队列
	select {
	case w.connChan <- wsConn:
	case <-w.closeChan:
		wsConn.Close()
	}
}

// Accept 接受 WebSocket 连接（服务端）
func (w *WebSocketAdapter) Accept() (io.ReadWriteCloser, error) {
	w.acceptMutex.Lock()
	defer w.acceptMutex.Unlock()

	select {
	case <-w.closeChan:
		return nil, coreerrors.New(coreerrors.CodeServiceClosed,
			"WebSocket adapter closed")
	case conn := <-w.connChan:
		return conn, nil
	case <-w.Ctx().Done():
		return nil, coreerrors.New(coreerrors.CodeCancelled,
			"context cancelled")
	}
}

// getConnectionType 返回连接类型
func (w *WebSocketAdapter) getConnectionType() string {
	return "WebSocket"
}

// onClose 清理资源
func (w *WebSocketAdapter) onClose() error {
	corelog.Info("WebSocketAdapter: closing...")

	var closeErr error
	w.closeOnce.Do(func() {
		close(w.closeChan)

		// 关闭 HTTP 服务器
		if w.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), WebSocketCloseTimeout)
			defer cancel()
			if err := w.httpServer.Shutdown(ctx); err != nil {
				corelog.Errorf("WebSocketAdapter: failed to shutdown HTTP server: %v", err)
				closeErr = err
			}
		}

		// 关闭监听器
		if w.listener != nil {
			if err := w.listener.Close(); err != nil {
				corelog.Errorf("WebSocketAdapter: failed to close listener: %v", err)
				if closeErr == nil {
					closeErr = err
				}
			}
			w.listener = nil
		}

		// 清空连接队列
		close(w.connChan)
		for conn := range w.connChan {
			conn.Close()
		}
	})

	// 调用基类清理
	baseErr := w.BaseAdapter.onClose()
	if closeErr == nil {
		closeErr = baseErr
	}

	corelog.Info("WebSocketAdapter: closed")
	return closeErr
}

// normalizeWSURL 规范化 WebSocket URL
func normalizeWSURL(addr string) string {
	// 如果已经是完整 URL，直接返回
	if len(addr) > 5 && (addr[:5] == "ws://" || addr[:6] == "wss://") {
		return addr
	}
	// 默认使用 ws:// 协议
	return "ws://" + addr + WebSocketDefaultPath
}

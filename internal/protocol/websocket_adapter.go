package protocol

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"github.com/gorilla/websocket"
)

const (
	// DefaultPingInterval 默认ping间隔时间
	DefaultPingInterval = 30 * time.Second
	// DefaultWriteTimeout 默认写入超时时间
	DefaultWriteTimeout = time.Second
)

// WebSocketConnWrapper WebSocket连接包装器，实现io.Reader和io.Writer接口
type WebSocketConnWrapper struct {
	conn *websocket.Conn
}

// Read 实现io.Reader接口
func (w *WebSocketConnWrapper) Read(p []byte) (n int, err error) {
	t, message, err := w.conn.ReadMessage()
	if err != nil {
		return 0, err
	}

	if t != websocket.BinaryMessage && t != websocket.TextMessage {
		return 0, nil
	}

	if len(message) > len(p) {
		return 0, fmt.Errorf("message too large for buffer")
	}

	copy(p, message)
	return len(message), nil
}

// Write 实现io.Writer接口
func (w *WebSocketConnWrapper) Write(p []byte) (n int, err error) {
	err = w.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// WebSocketAdapter WebSocket协议适配器
type WebSocketAdapter struct {
	BaseAdapter
	upgrader    websocket.Upgrader
	conn        *websocket.Conn
	server      *http.Server
	active      bool
	connMutex   sync.RWMutex
	stream      stream.PackageStreamer
	streamMutex sync.RWMutex
	session     *ConnectionSession
}

// NewWebSocketAdapter 创建新的WebSocket适配器
func NewWebSocketAdapter(parentCtx context.Context, session *ConnectionSession) *WebSocketAdapter {
	adapter := &WebSocketAdapter{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源，生产环境中应该更严格
			},
		},
		session: session,
	}
	adapter.SetName("websocket")
	adapter.SetCtx(parentCtx, adapter.onClose)
	return adapter
}

// ConnectTo 连接到WebSocket服务器
func (w *WebSocketAdapter) ConnectTo(serverAddr string) error {
	w.connMutex.Lock()
	defer w.connMutex.Unlock()

	if w.conn != nil {
		return fmt.Errorf("already connected")
	}

	// 确保地址以ws://或wss://开头
	if len(serverAddr) < 3 || (len(serverAddr) >= 3 && serverAddr[:3] != "ws:" && len(serverAddr) >= 4 && serverAddr[:4] != "wss:") {
		serverAddr = "ws://" + serverAddr
	}

	conn, _, err := websocket.DefaultDialer.Dial(serverAddr, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to websocket server: %w", err)
	}

	w.conn = conn
	w.SetAddr(serverAddr)

	// 创建数据流
	wrapper := &WebSocketConnWrapper{conn: conn}
	w.streamMutex.Lock()
	w.stream = stream.NewPackageStream(wrapper, wrapper, w.Ctx())
	w.streamMutex.Unlock()

	return nil
}

// ListenFrom 监听WebSocket连接
func (w *WebSocketAdapter) ListenFrom(serverAddr string) error {
	w.SetAddr(serverAddr)
	return nil
}

// Start 启动WebSocket适配器
func (w *WebSocketAdapter) Start(ctx context.Context) error {
	if w.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	// 如果已经连接，直接返回
	if w.conn != nil {
		return nil
	}

	// 创建HTTP服务器处理WebSocket升级
	mux := http.NewServeMux()
	mux.HandleFunc("/", w.handleWebSocket)

	// 解析地址 - 直接使用设置的地址，不添加协议前缀
	host := w.Addr()

	w.server = &http.Server{
		Addr:    host,
		Handler: mux,
	}

	w.active = true

	// 启动服务器
	go func() {
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if !w.IsClosed() {
				utils.Errorf("WebSocket server error: %v", err)
			}
		}
	}()

	return nil
}

// Stop 停止WebSocket适配器
func (w *WebSocketAdapter) Stop() error {
	w.active = false

	// 关闭HTTP服务器
	if w.server != nil {
		if err := w.server.Shutdown(context.Background()); err != nil {
			utils.Errorf("Failed to shutdown WebSocket server: %v", err)
		}
	}

	// 关闭WebSocket连接
	w.connMutex.Lock()
	if w.conn != nil {
		if err := w.conn.Close(); err != nil {
			utils.Errorf("Failed to close WebSocket connection: %v", err)
		}
		w.conn = nil
	}
	w.connMutex.Unlock()

	// 关闭数据流
	w.streamMutex.Lock()
	if w.stream != nil {
		w.stream.Close()
		w.stream = nil
	}
	w.streamMutex.Unlock()

	return nil
}

// GetReader 获取读取器
func (w *WebSocketAdapter) GetReader() io.Reader {
	w.streamMutex.RLock()
	defer w.streamMutex.RUnlock()

	if w.stream != nil {
		return w.stream.GetReader()
	}
	return nil
}

// GetWriter 获取写入器
func (w *WebSocketAdapter) GetWriter() io.Writer {
	w.streamMutex.RLock()
	defer w.streamMutex.RUnlock()

	if w.stream != nil {
		return w.stream.GetWriter()
	}
	return nil
}

// Close 关闭适配器
func (w *WebSocketAdapter) Close() {
	if err := w.Stop(); err != nil {
		utils.Errorf("Failed to stop WebSocket adapter: %v", err)
	}
	w.BaseAdapter.Close()
}

// handleWebSocket 处理WebSocket连接
func (w *WebSocketAdapter) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	// 升级HTTP连接为WebSocket
	conn, err := w.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		utils.Errorf("Failed to upgrade connection: %v", err)
		return
	}

	w.connMutex.Lock()
	w.conn = conn
	w.connMutex.Unlock()

	utils.Infof("WebSocket connection established from %s", conn.RemoteAddr())

	// 调用ConnectionSession.AcceptConnection处理连接
	if w.session != nil {
		wrapper := &WebSocketConnWrapper{conn: conn}
		w.session.AcceptConnection(wrapper, wrapper)
	} else {
		// 如果没有session，创建数据流并保持连接活跃
		wrapper := &WebSocketConnWrapper{conn: conn}
		w.streamMutex.Lock()
		w.stream = stream.NewPackageStream(wrapper, wrapper, w.Ctx())
		w.streamMutex.Unlock()

		// 保持连接活跃
		go w.keepAlive()
	}
}

// keepAlive 保持连接活跃
func (w *WebSocketAdapter) keepAlive() {
	w.connMutex.RLock()
	conn := w.conn
	w.connMutex.RUnlock()

	if conn == nil {
		return
	}

	// 设置ping处理器
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(DefaultWriteTimeout))
	})

	// 设置pong处理器
	conn.SetPongHandler(func(appData string) error {
		return nil
	})

	// 定期发送ping
	ticker := time.NewTicker(DefaultPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.connMutex.RLock()
			if w.conn != nil {
				if err := w.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(DefaultWriteTimeout)); err != nil {
					utils.Errorf("Failed to send ping: %v", err)
					w.connMutex.RUnlock()
					return
				}
			}
			w.connMutex.RUnlock()
		case <-w.Ctx().Done():
			return
		}
	}
}

// onClose 关闭时的清理函数
func (w *WebSocketAdapter) onClose() {
	if err := w.Stop(); err != nil {
		utils.Errorf("Failed to stop WebSocket adapter in onClose: %v", err)
	}
}

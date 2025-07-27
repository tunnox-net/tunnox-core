package adapter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/protocol/session"
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

// WebSocketConn WebSocket连接包装器
type WebSocketConn struct {
	conn *websocket.Conn
}

// Read 实现io.Reader接口
func (w *WebSocketConn) Read(p []byte) (n int, err error) {
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
func (w *WebSocketConn) Write(p []byte) (n int, err error) {
	err = w.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close 实现io.Closer接口
func (w *WebSocketConn) Close() error {
	if w.conn != nil {
		return w.conn.Close()
	}
	return nil
}

// WebSocketAdapter WebSocket协议适配器
// 只实现协议相关方法，其余继承 BaseAdapter
type WebSocketAdapter struct {
	BaseAdapter
	upgrader websocket.Upgrader
	server   *http.Server
}

func NewWebSocketAdapter(parentCtx context.Context, session session.Session) *WebSocketAdapter {
	w := &WebSocketAdapter{}
	w.BaseAdapter = BaseAdapter{} // 初始化 BaseAdapter
	w.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	w.SetName("websocket")
	w.SetSession(session)
	w.SetCtx(parentCtx, w.onClose)
	w.SetProtocolAdapter(w) // 设置协议适配器引用
	return w
}

func (w *WebSocketAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	if len(addr) < 3 || (len(addr) >= 3 && addr[:3] != "ws:" && len(addr) >= 4 && addr[:4] != "wss:") {
		addr = "ws://" + addr
	}
	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to websocket server: %w", err)
	}
	return &WebSocketConn{conn: conn}, nil
}

func (w *WebSocketAdapter) Listen(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", w.handleWebSocket)
	w.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	go func() {
		if w.server != nil {
			if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				if !w.IsClosed() {
					utils.Errorf("WebSocket server error: %v", err)
				}
			}
			utils.Infof("WebSocket server goroutine exited on %s", addr)
		}
	}()
	go func() {
		<-w.Ctx().Done()
		srv := w.server
		if srv != nil {
			utils.Infof("Shutting down WebSocket server on %s", addr)
			if err := srv.Shutdown(context.Background()); err != nil {
				utils.Errorf("Failed to shutdown WebSocket server: %v", err)
			}
		}
		utils.Infof("WebSocket shutdown goroutine exited on %s", addr)
	}()
	return nil
}

func (w *WebSocketAdapter) Accept() (io.ReadWriteCloser, error) {
	select {
	case <-time.After(100 * time.Millisecond):
		return nil, &TimeoutError{Protocol: "WebSocket connection"}
	}
}

func (w *WebSocketAdapter) getConnectionType() string {
	return "WebSocket"
}

// onClose WebSocket 特定的资源清理
func (w *WebSocketAdapter) onClose() error {
	// WebSocket upgrader 不需要显式关闭，只需要清理引用
	w.upgrader = websocket.Upgrader{}
	baseErr := w.BaseAdapter.onClose()
	return baseErr
}

// handleWebSocket 处理WebSocket连接
func (w *WebSocketAdapter) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	// 升级HTTP连接为WebSocket
	conn, err := w.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		utils.Errorf(constants.MsgFailedToUpgradeConnection, err)
		return
	}

	utils.Infof(constants.MsgWebSocketConnectionEstablished, conn.RemoteAddr())

	// 使用新的 Session 接口处理连接
	if w.GetSession() != nil {
		wrapper := &WebSocketConn{conn: conn}
		go func() {
			select {
			case <-w.Ctx().Done():
				utils.Infof(constants.MsgWebSocketHandlerExited, conn.RemoteAddr())
				return
			default:
				_, err := w.GetSession().AcceptConnection(wrapper, wrapper)
				if err != nil {
					utils.Errorf("Failed to initialize WebSocket connection: %v", err)
					return
				}
			}
		}()
	} else {
		// 如果没有session，创建数据流并保持连接活跃
		wrapper := &WebSocketConn{conn: conn}
		w.streamMutex.Lock()
		w.stream = stream.NewStreamProcessor(wrapper, wrapper, w.Ctx())
		w.streamMutex.Unlock()

		// 保持连接活跃
		go w.keepAlive()
		go func() {
			<-w.Ctx().Done()
			utils.Infof(constants.MsgWebSocketDefaultHandlerExited, conn.RemoteAddr())
			return
		}()
	}
}

// keepAlive 保持连接活跃
func (w *WebSocketAdapter) keepAlive() {
	ticker := time.NewTicker(DefaultPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.Ctx().Done():
			return
		case <-ticker.C:
			// 发送ping消息
			if w.stream != nil {
				if _, err := w.stream.WritePacket(nil, false, 0); err != nil {
					utils.Errorf("Failed to send ping: %v", err)
					return
				}
			}
		}
	}
}

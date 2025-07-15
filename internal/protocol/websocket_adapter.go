package protocol

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
	"tunnox-core/internal/constants"
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
type WebSocketAdapter struct {
	BaseAdapter
	upgrader websocket.Upgrader
	server   *http.Server
}

// NewWebSocketAdapter 创建新的WebSocket适配器
func NewWebSocketAdapter(parentCtx context.Context, session Session) *WebSocketAdapter {
	adapter := &WebSocketAdapter{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源，生产环境中应该更严格
			},
		},
	}
	adapter.SetName("websocket")
	adapter.SetSession(session)
	adapter.SetCtx(parentCtx, adapter.onClose)
	return adapter
}

// Dial 实现连接功能
func (w *WebSocketAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	// 确保地址以ws://或wss://开头
	if len(addr) < 3 || (len(addr) >= 3 && addr[:3] != "ws:" && len(addr) >= 4 && addr[:4] != "wss:") {
		addr = "ws://" + addr
	}

	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to websocket server: %w", err)
	}

	return &WebSocketConn{conn: conn}, nil
}

// Listen 实现监听功能
func (w *WebSocketAdapter) Listen(addr string) error {
	// 创建HTTP服务器处理WebSocket升级
	mux := http.NewServeMux()
	mux.HandleFunc("/", w.handleWebSocket)

	w.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// 启动服务器
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

	// 监听上下文取消，优雅关闭服务器
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

// Accept 实现接受连接功能
func (w *WebSocketAdapter) Accept() (io.ReadWriteCloser, error) {
	// WebSocket 监听器通过 HTTP 升级处理
	// 实际的连接在 handleWebSocket 中处理
	// 这里返回一个阻塞的虚拟连接，避免无限循环
	select {
	case <-time.After(100 * time.Millisecond): // 使用更短的超时时间
		return nil, &TimeoutError{Protocol: "WebSocket connection"}
	}
}

func (w *WebSocketAdapter) getConnectionType() string {
	return "WebSocket"
}

// 重写 ConnectTo 和 ListenFrom 以使用 BaseAdapter 的通用逻辑
func (w *WebSocketAdapter) ConnectTo(serverAddr string) error {
	return w.BaseAdapter.ConnectTo(w, serverAddr)
}

func (w *WebSocketAdapter) ListenFrom(listenAddr string) error {
	return w.BaseAdapter.ListenFrom(w, listenAddr)
}

// onClose WebSocket 特定的资源清理
func (w *WebSocketAdapter) onClose() error {
	var err error
	if w.server != nil {
		err = w.server.Shutdown(context.Background())
		w.server = nil
	}

	// 调用基类的清理方法
	baseErr := w.BaseAdapter.onClose()
	if err != nil {
		return err
	}
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
				// 初始化连接
				connInfo, err := w.GetSession().AcceptConnection(wrapper, wrapper)
				if err != nil {
					utils.Errorf("Failed to initialize WebSocket connection: %v", err)
					return
				}
				defer func(session Session, connectionId string) {
					err := session.CloseConnection(connectionId)
					if err != nil {
						utils.Errorf("Failed to close WebSocket connection: %v", err)
					}
				}(w.GetSession(), connInfo.ID)

				// 处理数据流
				for {
					packet, _, err := connInfo.Stream.ReadPacket()
					if err != nil {
						if err == io.EOF {
							utils.Infof("WebSocket connection closed by peer: %s", connInfo.ID)
						} else {
							utils.Errorf("Failed to read WebSocket packet: %v", err)
						}
						break
					}

					utils.Debugf("Received packet type: %v", packet.PacketType)

					// 包装成 StreamPacket
					connPacket := &StreamPacket{
						ConnectionID: connInfo.ID,
						Packet:       packet,
						Timestamp:    time.Now(),
					}

					// 处理数据包
					if err := w.GetSession().HandlePacket(connPacket); err != nil {
						utils.Errorf("Failed to handle WebSocket packet: %v", err)
						break
					}
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

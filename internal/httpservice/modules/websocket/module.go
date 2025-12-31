// Package websocket 提供 WebSocket 传输模块
// 用于客户端通过 WebSocket 方式连接服务器
package websocket

import (
	"context"
	"io"
	"net/http"
	"time"

	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// WebSocketModule WebSocket 传输模块
type WebSocketModule struct {
	*dispose.ServiceBase

	config   *httpservice.WebSocketModuleConfig
	deps     *httpservice.ModuleDependencies
	upgrader websocket.Upgrader
	connChan chan *WebSocketServerConn
	session  session.Session
}

// NewWebSocketModule 创建 WebSocket 模块
func NewWebSocketModule(ctx context.Context, config *httpservice.WebSocketModuleConfig) *WebSocketModule {
	m := &WebSocketModule{
		ServiceBase: dispose.NewService("WebSocketModule", ctx),
		config:      config,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  constants.WebSocketBufferSize,
			WriteBufferSize: constants.WebSocketBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins
			},
		},
		connChan: make(chan *WebSocketServerConn, 100),
	}

	return m
}

// Name 返回模块名称
func (m *WebSocketModule) Name() string {
	return "WebSocket"
}

// SetDependencies 注入依赖
func (m *WebSocketModule) SetDependencies(deps *httpservice.ModuleDependencies) {
	m.deps = deps
}

// SetSession 设置会话管理器
func (m *WebSocketModule) SetSession(sess session.Session) {
	m.session = sess
}

// RegisterRoutes 注册路由
func (m *WebSocketModule) RegisterRoutes(router *mux.Router) {
	if !m.config.Enabled {
		corelog.Infof("WebSocketModule: disabled, skipping route registration")
		return
	}

	// 注册 /_tunnox 路由处理 WebSocket 升级
	router.HandleFunc("/_tunnox", m.handleWebSocket).Methods("GET")
	corelog.Infof("WebSocketModule: registered route /_tunnox")
}

// handleWebSocket 处理 WebSocket 升级请求
func (m *WebSocketModule) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 升级 HTTP 连接为 WebSocket
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		corelog.Errorf("WebSocketModule: upgrade failed: %v", err)
		return
	}

	// 包装 WebSocket 连接
	wsConn := &WebSocketServerConn{
		conn:           conn,
		remoteAddr:     r.RemoteAddr,
		closed:         make(chan struct{}),
		streamDataChan: make(chan []byte, 100), // 流模式数据通道
	}

	// 如果有会话管理器，处理连接
	if m.session != nil {
		go m.handleConnection(wsConn)
	} else {
		corelog.Warnf("WebSocketModule: session is nil, cannot handle connection")
		// 没有会话管理器，发送到通道等待处理
		select {
		case m.connChan <- wsConn:
		case <-m.Ctx().Done():
			wsConn.Close()
			return
		case <-time.After(5 * time.Second):
			corelog.Errorf("WebSocketModule: connection queue full, rejecting")
			wsConn.Close()
			return
		}
	}
}

// handleConnection 处理 WebSocket 连接的数据包循环
func (m *WebSocketModule) handleConnection(wsConn *WebSocketServerConn) {
	shouldCloseConn := true
	var streamConn *types.StreamConnection

	defer func() {
		// 清理 SessionManager 中的连接（如果已创建，忽略关闭错误，连接可能已关闭）
		if streamConn != nil && m.session != nil {
			_ = m.session.CloseConnection(streamConn.ID)
		}

		// 关闭底层连接
		if shouldCloseConn {
			wsConn.Close()
		}
	}()

	// 初始化连接
	var err error
	streamConn, err = m.session.AcceptConnection(wsConn, wsConn)
	if err != nil {
		corelog.Errorf("WebSocketModule: failed to accept connection: %v", err)
		return
	}

	// 设置连接ID（用于调试）
	wsConn.SetConnectionID(streamConn.ID)

	// 数据包处理循环
	for {
		select {
		case <-m.Ctx().Done():
			return
		default:
		}

		// 检查连接是否已切换到流模式
		if streamConn != nil && streamConn.Stream != nil {
			if reader, ok := streamConn.Stream.GetReader().(interface {
				IsStreamMode() bool
			}); ok && reader.IsStreamMode() {
				// 启动流模式读取器
				wsConn.SetStreamMode(true)
				wsConn.StartStreamModeReader()
				shouldCloseConn = false
				streamConn = nil
				return
			}
		}

		pkt, _, err := streamConn.Stream.ReadPacket()
		if err != nil {
			// 检查是否为超时错误
			if netErr, ok := err.(interface {
				Timeout() bool
				Temporary() bool
			}); ok && netErr.Timeout() && netErr.Temporary() {
				continue
			}
			if err != io.EOF {
				corelog.Errorf("WebSocketModule: failed to read packet for connection %s: %v", streamConn.ID, err)
			}
			return
		}

		streamPacket := &types.StreamPacket{
			ConnectionID: streamConn.ID,
			Packet:       pkt,
			Timestamp:    time.Now(),
		}

		isTunnelOpenPacket := (pkt.PacketType & 0x3F) == packet.TunnelOpen

		if err := m.session.HandlePacket(streamPacket); err != nil {
			if isTunnelOpenPacket && coreerrors.IsCode(err, coreerrors.CodeTunnelModeSwitch) {
				// 启动流模式读取器
				wsConn.SetStreamMode(true)
				wsConn.StartStreamModeReader()
				shouldCloseConn = false
				streamConn = nil
				return
			}
			corelog.Errorf("WebSocketModule: failed to handle packet for connection %s: %v", streamConn.ID, err)
		}
	}
}

// Start 启动模块
func (m *WebSocketModule) Start() error {
	if !m.config.Enabled {
		corelog.Infof("WebSocketModule: disabled")
		return nil
	}
	corelog.Infof("WebSocketModule: started")
	return nil
}

// Stop 停止模块
func (m *WebSocketModule) Stop() error {
	close(m.connChan)
	corelog.Infof("WebSocketModule: stopped")
	return nil
}

// GetConnChan 获取连接通道（供外部处理连接）
func (m *WebSocketModule) GetConnChan() <-chan *WebSocketServerConn {
	return m.connChan
}

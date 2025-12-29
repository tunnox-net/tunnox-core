// Package domainproxy 提供 HTTP 域名代理功能
package domainproxy

import (
	"fmt"
	"io"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"

	"github.com/gorilla/websocket"
)

// forwardWebSocket 双向转发 WebSocket 数据
func (m *DomainProxyModule) forwardWebSocket(userWS *websocket.Conn, tunnelConn httpservice.TunnelConnectionInterface) {
	// 创建错误通道
	errChan := make(chan error, 2)

	// 用户 -> 隧道
	go m.forwardUserToTunnel(userWS, tunnelConn, errChan)

	// 隧道 -> 用户
	go m.forwardTunnelToUser(tunnelConn, userWS, errChan)

	// 等待任一方向出错或关闭
	err := <-errChan
	if err != nil && err != io.EOF {
		corelog.Debugf("DomainProxyModule: WebSocket forwarding stopped: %v", err)
	}

	// 关闭连接
	userWS.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second))
}

// forwardUserToTunnel 转发用户 WebSocket 消息到隧道
func (m *DomainProxyModule) forwardUserToTunnel(userWS *websocket.Conn, tunnelConn httpservice.TunnelConnectionInterface, errChan chan error) {
	defer func() {
		if r := recover(); r != nil {
			corelog.Errorf("DomainProxyModule: panic in forwardUserToTunnel: %v", r)
			errChan <- fmt.Errorf("panic: %v", r)
		}
	}()

	for {
		// 读取 WebSocket 消息
		messageType, data, err := userWS.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				corelog.Debugf("DomainProxyModule: user WebSocket closed normally")
				errChan <- io.EOF
			} else {
				corelog.Errorf("DomainProxyModule: failed to read from user WebSocket: %v", err)
				errChan <- err
			}
			return
		}

		// 写入隧道（格式：1字节类型 + 数据）
		frame := make([]byte, 1+len(data))
		frame[0] = byte(messageType)
		copy(frame[1:], data)

		if _, err := tunnelConn.Write(frame); err != nil {
			corelog.Errorf("DomainProxyModule: failed to write to tunnel: %v", err)
			errChan <- err
			return
		}

		corelog.Debugf("DomainProxyModule: forwarded %d bytes from user to tunnel (type=%d)", len(data), messageType)
	}
}

// forwardTunnelToUser 转发隧道消息到用户 WebSocket
func (m *DomainProxyModule) forwardTunnelToUser(tunnelConn httpservice.TunnelConnectionInterface, userWS *websocket.Conn, errChan chan error) {
	defer func() {
		if r := recover(); r != nil {
			corelog.Errorf("DomainProxyModule: panic in forwardTunnelToUser: %v", r)
			errChan <- fmt.Errorf("panic: %v", r)
		}
	}()

	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		// 读取隧道数据（格式：1字节类型 + 数据）
		n, err := tunnelConn.Read(buf)
		if err != nil {
			if err == io.EOF {
				corelog.Debugf("DomainProxyModule: tunnel closed")
				errChan <- io.EOF
			} else {
				corelog.Errorf("DomainProxyModule: failed to read from tunnel: %v", err)
				errChan <- err
			}
			return
		}

		if n < 1 {
			continue
		}

		// 解析消息类型和数据
		messageType := int(buf[0])
		data := buf[1:n]

		// 写入用户 WebSocket
		if err := userWS.WriteMessage(messageType, data); err != nil {
			corelog.Errorf("DomainProxyModule: failed to write to user WebSocket: %v", err)
			errChan <- err
			return
		}

		corelog.Debugf("DomainProxyModule: forwarded %d bytes from tunnel to user (type=%d)", len(data), messageType)
	}
}

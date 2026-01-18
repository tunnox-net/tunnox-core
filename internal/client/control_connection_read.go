package client

import (
	"io"
	"runtime/debug"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// readLoop 读取循环（接收服务器命令）
func (c *TunnoxClient) readLoop() {
	defer func() {
		// 🔥 Panic recovery - 捕获并记录 readLoop 中的 panic
		if r := recover(); r != nil {
			corelog.Errorf("FATAL: readLoop panic recovered: %v", r)
			corelog.Errorf("Stack trace:\n%s", string(debug.Stack()))
		}

		if c.shouldReconnect() {
			go c.reconnect()
		}
	}()

	for {
		select {
		case <-c.Ctx().Done():
			return
		default:
		}

		pkt, _, err := c.controlStream.ReadPacket()
		if err != nil {
			if err != io.EOF {
				corelog.Errorf("Client: failed to read packet: %v", err)
			}
			c.mu.Lock()
			if c.controlStream != nil {
				c.controlStream.Close()
				c.controlStream = nil
			}
			if c.controlConn != nil {
				c.controlConn.Close()
				c.controlConn = nil
			}
			c.mu.Unlock()
			return
		}

		switch pkt.PacketType & 0x3F {
		case packet.Heartbeat:
		case packet.CommandResp:
			if c.commandResponseManager != nil && c.commandResponseManager.HandleResponse(pkt) {
				continue
			}
		case packet.JsonCommand:
			c.handleCommand(pkt)
		case packet.TunnelOpen:
			// ✅ TunnelOpen 应该由隧道连接处理，控制连接忽略它
			corelog.Debugf("Client: ignoring TunnelOpen in control connection read loop")
		case packet.TunnelOpenAck:
			// ✅ TunnelOpenAck 应该由隧道连接处理，控制连接忽略它
			corelog.Debugf("Client: ignoring TunnelOpenAck in control connection read loop")
		default:
			corelog.Warnf("Client: unknown packet type: %d", pkt.PacketType)
		}
	}
}

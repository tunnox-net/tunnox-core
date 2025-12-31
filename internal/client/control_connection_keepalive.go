package client

import (
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// heartbeatLoop 心跳循环
func (c *TunnoxClient) heartbeatLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx().Done():
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				corelog.Errorf("Client: failed to send heartbeat: %v", err)
				return
			}
		}
	}
}

// sendHeartbeat 发送心跳包
func (c *TunnoxClient) sendHeartbeat() error {
	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		return coreerrors.New(coreerrors.CodeConnectionError, "control stream is nil")
	}

	heartbeatPkt := &packet.TransferPacket{
		PacketType: packet.Heartbeat,
		Payload:    []byte{},
	}
	_, err := controlStream.WritePacket(heartbeatPkt, true, 0)
	if err != nil {
		// 心跳失败，可能连接已断开，清理连接状态
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
	}
	return err
}

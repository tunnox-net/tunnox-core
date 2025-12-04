package client

import (
	"io"
	"time"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// readLoop 读取循环（接收服务器命令）
func (c *TunnoxClient) readLoop() {
	defer func() {
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
				utils.Errorf("Client: failed to read packet: %v", err)
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
			utils.Debugf("Client: ignoring TunnelOpen in control connection read loop")
		case packet.TunnelOpenAck:
			// ✅ TunnelOpenAck 应该由隧道连接处理，控制连接忽略它
			utils.Debugf("Client: ignoring TunnelOpenAck in control connection read loop")
		default:
			utils.Warnf("Client: unknown packet type: %d", pkt.PacketType)
		}
	}
}

// requestMappingConfig 请求当前客户端的映射配置
func (c *TunnoxClient) requestMappingConfig() {
	if !c.configRequesting.CompareAndSwap(false, true) {
		return
	}
	defer c.configRequesting.Store(false)

	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		return
	}

	commandID, err := utils.GenerateRandomString(16)
	if err != nil {
		utils.Errorf("Client: failed to generate command ID: %v", err)
		return
	}

	responseChan := c.commandResponseManager.RegisterRequest(commandID)
	defer c.commandResponseManager.UnregisterRequest(commandID)

	cmd := &packet.CommandPacket{
		CommandType: packet.ConfigGet,
		CommandBody: "{}",
		CommandId:   commandID,
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	// 重试发送请求（最多3次，每次间隔1秒）
	var writeErr error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			time.Sleep(time.Second)
			utils.Debugf("Client: retrying mapping config request (attempt %d/3)", retry+1)
		}
		_, writeErr = controlStream.WritePacket(pkt, true, 0)
		if writeErr == nil {
			break
		}
		utils.Warnf("Client: failed to request mapping config (attempt %d/3): %v", retry+1, writeErr)
	}

	if writeErr != nil {
		utils.Errorf("Client: failed to request mapping config after 3 attempts: %v", writeErr)
		return
	}

	select {
	case resp := <-responseChan:
		if !resp.Success {
			utils.Errorf("Client: ConfigGet failed: %s", resp.Error)
			return
		}
		c.handleConfigUpdate(resp.Data)
	case <-time.After(30 * time.Second):
		utils.Errorf("Client: ConfigGet request timeout after 30s")
	case <-c.Ctx().Done():
	}
}


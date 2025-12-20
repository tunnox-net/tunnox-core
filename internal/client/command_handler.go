package client

import (
	"encoding/json"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/packet"
)

// handleCommand 处理命令
func (c *TunnoxClient) handleCommand(pkt *packet.TransferPacket) {
	if pkt.CommandPacket == nil {
		corelog.Warnf("Client: received command packet with nil CommandPacket")
		return
	}

	cmdType := pkt.CommandPacket.CommandType
	corelog.Debugf("Client: received command, type=%v", cmdType)

	switch cmdType {
	case packet.ConfigSet:
		// 接收服务器推送的配置
		c.handleConfigUpdate(pkt.CommandPacket.CommandBody)

	case packet.TunnelOpenRequestCmd:
		// 隧道打开请求（作为目标客户端）
		c.handleTunnelOpenRequest(pkt.CommandPacket.CommandBody)

	case packet.KickClient:
		// 踢下线命令
		c.handleKickCommand(pkt.CommandPacket.CommandBody)

	case packet.HTTPProxyRequest:
		// HTTP 代理请求
		c.handleHTTPProxyRequest(pkt.CommandPacket)
	}
}

// handleHTTPProxyRequest 处理 HTTP 代理请求
func (c *TunnoxClient) handleHTTPProxyRequest(cmd *packet.CommandPacket) {
	// 解析请求
	var req httpservice.HTTPProxyRequest
	if err := json.Unmarshal([]byte(cmd.CommandBody), &req); err != nil {
		corelog.Errorf("Client: failed to parse HTTP proxy request: %v", err)
		c.sendHTTPProxyErrorResponse(cmd.CommandId, "invalid request format")
		return
	}

	corelog.Debugf("Client: handling HTTP proxy request %s, method=%s, url=%s",
		req.RequestID, req.Method, req.URL)

	// 获取或创建 HTTP 代理执行器
	executor := c.getHTTPProxyExecutor()
	if executor == nil {
		corelog.Errorf("Client: HTTP proxy executor not available")
		c.sendHTTPProxyErrorResponse(cmd.CommandId, "HTTP proxy not available")
		return
	}

	// 执行代理请求
	resp, err := executor.Execute(&req)
	if err != nil {
		corelog.Warnf("Client: HTTP proxy request failed: %v", err)
		c.sendHTTPProxyErrorResponse(cmd.CommandId, err.Error())
		return
	}

	// 发送响应
	c.sendHTTPProxyResponse(cmd.CommandId, resp)
}

// sendHTTPProxyResponse 发送 HTTP 代理响应
func (c *TunnoxClient) sendHTTPProxyResponse(commandID string, resp *httpservice.HTTPProxyResponse) {
	respBody, err := json.Marshal(resp)
	if err != nil {
		corelog.Errorf("Client: failed to marshal HTTP proxy response: %v", err)
		return
	}

	respPkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.HTTPProxyResponse,
			CommandId:   commandID,
			CommandBody: string(respBody),
		},
	}

	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		corelog.Errorf("Client: control stream is nil, cannot send HTTP proxy response")
		return
	}

	if _, err := controlStream.WritePacket(respPkt, true, 0); err != nil {
		corelog.Errorf("Client: failed to send HTTP proxy response: %v", err)
	} else {
		corelog.Debugf("Client: sent HTTP proxy response for request %s, status=%d",
			resp.RequestID, resp.StatusCode)
	}
}

// sendHTTPProxyErrorResponse 发送 HTTP 代理错误响应
func (c *TunnoxClient) sendHTTPProxyErrorResponse(commandID string, errMsg string) {
	resp := &httpservice.HTTPProxyResponse{
		RequestID: commandID,
		Error:     errMsg,
	}
	c.sendHTTPProxyResponse(commandID, resp)
}

// handleKickCommand 处理踢下线命令
func (c *TunnoxClient) handleKickCommand(cmdBody string) {
	var kickInfo struct {
		Reason string `json:"reason"`
		Code   string `json:"code"`
	}

	if err := json.Unmarshal([]byte(cmdBody), &kickInfo); err != nil {
		corelog.Errorf("Client: failed to parse kick command: %v", err)
		kickInfo.Reason = "Unknown reason"
		kickInfo.Code = "UNKNOWN"
	}

	corelog.Errorf("Client: KICKED BY SERVER - Reason: %s, Code: %s", kickInfo.Reason, kickInfo.Code)

	// 标记为被踢下线，禁止重连
	c.kicked = true

	// 停止客户端
	c.Stop()
}

// getHTTPProxyExecutor 获取 HTTP 代理执行器
func (c *TunnoxClient) getHTTPProxyExecutor() *HTTPProxyExecutor {
	// 使用默认配置创建执行器
	return NewHTTPProxyExecutor(nil)
}

package client

import (
	"encoding/json"
	"net"

	"tunnox-core/internal/client/notify"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/httptypes"
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

	case packet.NotifyClient:
		// 服务端推送的通知
		c.handleNotification(pkt.CommandPacket.CommandBody)

	case packet.DNSResolve:
		// DNS 解析请求（作为 targetClient）
		c.handleDNSResolveRequest(pkt.CommandPacket)
	}
}

// handleHTTPProxyRequest 处理 HTTP 代理请求
func (c *TunnoxClient) handleHTTPProxyRequest(cmd *packet.CommandPacket) {
	// 解析请求
	var req httptypes.HTTPProxyRequest
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
func (c *TunnoxClient) sendHTTPProxyResponse(commandID string, resp *httptypes.HTTPProxyResponse) {
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
	resp := &httptypes.HTTPProxyResponse{
		RequestID: commandID,
		Error:     errMsg,
	}
	c.sendHTTPProxyResponse(commandID, resp)
}

// handleKickCommand 处理踢下线命令
//
// 特殊 Code 处理：
// - "credentials_reset": 凭据被重置，客户端需要使用新凭据重新配置
// - "auth_failed": 认证失败（凭据过期或无效）
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

	// 根据 Code 设置不同的标志
	switch kickInfo.Code {
	case "credentials_reset":
		// 凭据被重置：客户端将退出，需要用户使用新凭据
		c.credentialsReset = true
		corelog.Errorf("Client: credentials have been reset, please obtain the new SecretKey from the server")
	case "auth_failed", "expired":
		// 认证失败或凭据过期：客户端将退出
		c.authFailed = true
		corelog.Errorf("Client: authentication failed or credentials expired")
	default:
		// 其他原因（如管理员踢下线）：禁止重连但不退出
		c.kicked = true
	}

	// 停止客户端
	c.Stop()
}

// getHTTPProxyExecutor 获取 HTTP 代理执行器
func (c *TunnoxClient) getHTTPProxyExecutor() *HTTPProxyExecutor {
	// 使用默认配置创建执行器
	return NewHTTPProxyExecutor(nil)
}

// handleNotification 处理服务端推送的通知
func (c *TunnoxClient) handleNotification(cmdBody string) {
	var notification packet.ClientNotification
	if err := json.Unmarshal([]byte(cmdBody), &notification); err != nil {
		corelog.Errorf("Client: failed to parse notification: %v", err)
		return
	}

	corelog.Debugf("Client: received notification id=%s, type=%s, sender=%d",
		notification.NotifyID, notification.Type.String(), notification.SenderClientID)

	// 检查是否过期
	if notification.IsExpired() {
		corelog.Warnf("Client: notification %s has expired, ignoring", notification.NotifyID)
		return
	}

	// 分发通知到处理器
	if c.notificationDispatcher != nil {
		c.notificationDispatcher.Dispatch(&notification)
	}

	// 如果需要确认，发送确认响应
	if notification.RequireAck {
		c.sendNotificationAck(notification.NotifyID, true, true, "")
	}
}

// sendNotificationAck 发送通知确认
func (c *TunnoxClient) sendNotificationAck(notifyID string, received, processed bool, errMsg string) {
	ackReq := &packet.NotifyAckRequest{
		NotifyID:  notifyID,
		Received:  received,
		Processed: processed,
		Error:     errMsg,
	}

	ackBody, err := json.Marshal(ackReq)
	if err != nil {
		corelog.Errorf("Client: failed to marshal notification ack: %v", err)
		return
	}

	ackPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.NotifyClientAck,
			CommandBody: string(ackBody),
		},
	}

	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		corelog.Errorf("Client: control stream is nil, cannot send notification ack")
		return
	}

	if _, err := controlStream.WritePacket(ackPkt, true, 0); err != nil {
		corelog.Errorf("Client: failed to send notification ack: %v", err)
	} else {
		corelog.Debugf("Client: sent notification ack for %s", notifyID)
	}
}

// AddNotificationHandler 添加通知处理器
func (c *TunnoxClient) AddNotificationHandler(handler notify.Handler) {
	if c.notificationDispatcher != nil {
		c.notificationDispatcher.AddHandler(handler)
	}
}

// RemoveNotificationHandler 移除通知处理器
func (c *TunnoxClient) RemoveNotificationHandler(handler notify.Handler) {
	if c.notificationDispatcher != nil {
		c.notificationDispatcher.RemoveHandler(handler)
	}
}

// SetDefaultNotificationHandler 设置默认通知处理器（记录日志）
func (c *TunnoxClient) SetDefaultNotificationHandler() {
	c.AddNotificationHandler(&notify.DefaultHandler{})
}

// handleDNSResolveRequest 处理 DNS 解析请求
// 当本客户端作为 targetClient 时，接收 DNS 解析请求并使用本地系统 DNS 进行解析
func (c *TunnoxClient) handleDNSResolveRequest(cmd *packet.CommandPacket) {
	var req packet.DNSResolveRequest
	if err := json.Unmarshal([]byte(cmd.CommandBody), &req); err != nil {
		corelog.Errorf("Client: failed to parse DNS resolve request: %v", err)
		c.sendDNSResolveResponse(cmd.CommandId, nil, err.Error())
		return
	}

	corelog.Debugf("Client: handling DNS resolve request for %s (qtype=%d)", req.Domain, req.QType)

	// 使用系统 DNS 解析
	addrs, err := net.LookupHost(req.Domain)
	if err != nil {
		corelog.Warnf("Client: DNS resolve failed for %s: %v", req.Domain, err)
		c.sendDNSResolveResponse(cmd.CommandId, nil, err.Error())
		return
	}

	// 根据 qtype 过滤 IP
	var ips []string
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		isIPv4 := ip.To4() != nil
		if req.QType == 1 && isIPv4 { // A record
			ips = append(ips, addr)
		} else if req.QType == 28 && !isIPv4 { // AAAA record
			ips = append(ips, addr)
		}
	}

	// 如果请求的类型没有结果，但有其他类型的结果，也返回
	if len(ips) == 0 && len(addrs) > 0 {
		ips = addrs
	}

	corelog.Debugf("Client: DNS resolved %s -> %v", req.Domain, ips)
	c.sendDNSResolveResponse(cmd.CommandId, ips, "")
}

// sendDNSResolveResponse 发送 DNS 解析响应
func (c *TunnoxClient) sendDNSResolveResponse(commandID string, ips []string, errMsg string) {
	resp := &packet.DNSResolveResponse{
		Success: errMsg == "",
		IPs:     ips,
		TTL:     300,
		Error:   errMsg,
	}

	respBody, err := json.Marshal(resp)
	if err != nil {
		corelog.Errorf("Client: failed to marshal DNS resolve response: %v", err)
		return
	}

	respPkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.DNSResolve,
			CommandId:   commandID,
			CommandBody: string(respBody),
		},
	}

	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		corelog.Errorf("Client: control stream is nil, cannot send DNS resolve response")
		return
	}

	if _, err := controlStream.WritePacket(respPkt, true, 0); err != nil {
		corelog.Errorf("Client: failed to send DNS resolve response: %v", err)
	} else {
		corelog.Debugf("Client: sent DNS resolve response for %s", commandID)
	}
}

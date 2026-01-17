package client

import (
	"context"
	"encoding/json"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ResolveDNS 通过隧道解析 DNS
// 将 DNS 请求发送到服务器，由服务器转发给 targetClient 进行解析
// req: DNS 解析请求
// timeout: 超时时间
// 返回: DNS 解析响应，错误信息
func (c *TunnoxClient) ResolveDNS(req *packet.DNSResolveRequest, timeout time.Duration) (*packet.DNSResolveResponse, string) {
	if !c.IsConnected() {
		return nil, "not connected to server"
	}

	// 创建带超时的 context
	ctx, cancel := context.WithTimeout(c.Ctx(), timeout)
	defer cancel()

	// 发送命令并等待响应
	cmdResp, err := c.sendCommandAndWaitResponseWithContext(ctx, &CommandRequest{
		CommandType: packet.DNSResolve,
		RequestBody: req,
		EnableTrace: false,
	})

	if err != nil {
		corelog.Warnf("Client: DNS resolve failed for %s: %v", req.Domain, err)
		return nil, err.Error()
	}

	// 解析响应
	var resp packet.DNSResolveResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		corelog.Errorf("Client: failed to parse DNS response: %v", err)
		return nil, "failed to parse DNS response"
	}

	return &resp, ""
}

// ResolveDNSSimple 简化版 DNS 解析（A 记录）
// domain: 要解析的域名
// targetClientID: 目标客户端 ID（-1 表示使用默认目标）
// 返回: 第一个 IP 地址，错误信息
func (c *TunnoxClient) ResolveDNSSimple(domain string, targetClientID int64) (string, string) {
	req := &packet.DNSResolveRequest{
		Domain:         domain,
		QType:          1, // A record
		TargetClientID: targetClientID,
	}

	resp, errMsg := c.ResolveDNS(req, 5*time.Second)
	if errMsg != "" {
		return "", errMsg
	}

	if !resp.Success {
		return "", resp.Error
	}

	if len(resp.IPs) == 0 {
		return "", "no IP addresses found"
	}

	return resp.IPs[0], ""
}

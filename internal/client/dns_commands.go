package client

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

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

// QueryDNS 通过控制通道发送原始 DNS 查询
// 实现 socks5.DNSQueryHandler 接口
// targetClientID: 目标客户端 ID（执行 DNS 查询的客户端）
// dnsServer: DNS 服务器地址（如 "119.29.29.29:53"）
// rawQuery: 原始 DNS 查询报文
// 返回: 原始 DNS 响应报文
func (c *TunnoxClient) QueryDNS(targetClientID int64, dnsServer string, rawQuery []byte) ([]byte, error) {
	corelog.Infof("Client: QueryDNS called, targetClientID=%d, dnsServer=%s, queryLen=%d, isConnected=%v",
		targetClientID, dnsServer, len(rawQuery), c.IsConnected())

	if !c.IsConnected() {
		corelog.Warnf("Client: QueryDNS failed - not connected to server")
		return nil, &dnsError{msg: "not connected to server"}
	}

	// 生成唯一查询ID
	queryID := uuid.New().String()

	req := &packet.DNSQueryRequest{
		QueryID:        queryID,
		TargetClientID: targetClientID,
		DNSServer:      dnsServer,
		RawQuery:       rawQuery,
	}

	// 创建带超时的 context (DNS 查询应该快速完成)
	ctx, cancel := context.WithTimeout(c.Ctx(), 5*time.Second)
	defer cancel()

	corelog.Infof("Client: sending DNS query via control channel, queryID=%s, target=%d, server=%s, queryLen=%d",
		queryID, targetClientID, dnsServer, len(rawQuery))

	// 发送命令并等待响应
	cmdResp, err := c.sendCommandAndWaitResponseWithContext(ctx, &CommandRequest{
		CommandType: packet.DNSQuery,
		RequestBody: req,
		EnableTrace: false,
	})

	if err != nil {
		corelog.Warnf("Client: DNS query failed, queryID=%s: %v", queryID, err)
		return nil, &dnsError{msg: err.Error()}
	}

	// 解析响应
	var resp packet.DNSQueryResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		corelog.Errorf("Client: failed to parse DNS query response: %v", err)
		return nil, &dnsError{msg: "failed to parse DNS response"}
	}

	if !resp.Success {
		corelog.Warnf("Client: DNS query returned error, queryID=%s: %s", queryID, resp.Error)
		return nil, &dnsError{msg: resp.Error}
	}

	corelog.Infof("Client: DNS query success via control channel, queryID=%s, responseLen=%d",
		queryID, len(resp.RawAnswer))

	return resp.RawAnswer, nil
}

// dnsError DNS 查询错误
type dnsError struct {
	msg string
}

func (e *dnsError) Error() string {
	return e.msg
}

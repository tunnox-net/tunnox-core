package mobile

import (
	"errors"
	"strings"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ResolveDNS 通过 tunnox 隧道解析 DNS
// domain: 要解析的域名
// qtype: 查询类型 (1=A 记录, 28=AAAA 记录)
// targetClientID: 目标客户端 ID（-1 表示使用默认目标）
// 返回: IP 地址列表（逗号分隔），错误
func (c *TunnoxMobileClient) ResolveDNS(domain string, qtype int64, targetClientID int64) (string, error) {
	if !c.IsConnected() {
		return "", errors.New("not connected to server")
	}

	// 构造 DNS 解析请求
	req := &packet.DNSResolveRequest{
		Domain:         domain,
		QType:          int(qtype),
		TargetClientID: targetClientID,
	}

	// 发送 DNS 解析请求到服务器
	resp, errMsg := c.client.ResolveDNS(req, 5*time.Second)
	if errMsg != "" {
		corelog.Warnf("DNS resolve failed for %s: %s", domain, errMsg)
		return "", errors.New(errMsg)
	}

	if !resp.Success {
		return "", errors.New(resp.Error)
	}

	// 返回 IP 列表（逗号分隔）
	return strings.Join(resp.IPs, ","), nil
}

// ResolveDNSSimple 简化版 DNS 解析（使用默认目标客户端，A 记录）
// domain: 要解析的域名
// 返回: 第一个 IP 地址，错误
func (c *TunnoxMobileClient) ResolveDNSSimple(domain string) (string, error) {
	ips, err := c.ResolveDNS(domain, 1, -1)
	if err != nil {
		return "", err
	}
	if ips == "" {
		return "", errors.New("no IP addresses found")
	}
	// 返回第一个 IP
	parts := strings.Split(ips, ",")
	return strings.TrimSpace(parts[0]), nil
}

package command

import (
	"encoding/json"
	"net"
	"strings"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// DNSResolveHandler DNS 解析处理器
// 在 targetClient 侧运行，接收 DNS 解析请求并使用本地系统 DNS 进行解析
type DNSResolveHandler struct {
	*BaseHandler
}

// NewDNSResolveHandler 创建 DNS 解析处理器
func NewDNSResolveHandler() *DNSResolveHandler {
	return &DNSResolveHandler{
		BaseHandler: NewBaseHandler(
			packet.DNSResolve,
			CategoryManagement,
			DirectionDuplex,
			"dns_resolve",
			"DNS 解析（使用本地系统 DNS）",
		),
	}
}

// Handle 处理 DNS 解析请求
func (h *DNSResolveHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	// 解析请求
	var req packet.DNSResolveRequest
	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		corelog.Errorf("Failed to parse DNS resolve request: %v", err)
		return h.errorResponse(ctx, "invalid request format")
	}

	corelog.Debugf("DNS resolve request: domain=%s, qtype=%d", req.Domain, req.QType)

	// 使用系统 DNS 解析
	addrs, err := net.LookupHost(req.Domain)
	if err != nil {
		corelog.Warnf("DNS resolve failed for %s: %v", req.Domain, err)
		return h.errorResponse(ctx, err.Error())
	}

	// 根据 qtype 过滤 IP
	var ips []string
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		isIPv4 := ip.To4() != nil
		if req.QType == 1 && isIPv4 { // A record (IPv4)
			ips = append(ips, addr)
		} else if req.QType == 28 && !isIPv4 { // AAAA record (IPv6)
			ips = append(ips, addr)
		}
	}

	// 如果请求的类型没有结果，但有其他类型的结果，也返回
	if len(ips) == 0 && len(addrs) > 0 {
		ips = addrs
	}

	corelog.Debugf("DNS resolved %s -> %v", req.Domain, ips)

	// 构造响应
	resp := packet.DNSResolveResponse{
		Success: true,
		IPs:     ips,
		TTL:     300, // 默认 TTL 5 分钟
	}

	data, _ := json.Marshal(resp)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// errorResponse 构造错误响应
func (h *DNSResolveHandler) errorResponse(ctx *CommandContext, errMsg string) (*CommandResponse, error) {
	resp := packet.DNSResolveResponse{
		Success: false,
		Error:   errMsg,
	}
	data, _ := json.Marshal(resp)
	return &CommandResponse{
		Success:   false,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// ResolveDNS 直接解析 DNS（供 mobile 包调用）
// domain: 要解析的域名
// qtype: 查询类型 (1=A, 28=AAAA)
// 返回: IP 地址列表（逗号分隔）或错误
func ResolveDNS(domain string, qtype int) (string, error) {
	addrs, err := net.LookupHost(domain)
	if err != nil {
		return "", err
	}

	var ips []string
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		isIPv4 := ip.To4() != nil
		if qtype == 1 && isIPv4 {
			ips = append(ips, addr)
		} else if qtype == 28 && !isIPv4 {
			ips = append(ips, addr)
		}
	}

	if len(ips) == 0 && len(addrs) > 0 {
		ips = addrs
	}

	return strings.Join(ips, ","), nil
}

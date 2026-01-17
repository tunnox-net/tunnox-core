package mobile

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	corelog "tunnox-core/internal/core/log"
)

// DNSServer 本地 DNS 服务器
// 监听 UDP 端口，接收 DNS 查询，通过 TunnoxMobileClient 解析
type DNSServer struct {
	client     *TunnoxMobileClient
	listenAddr string
	conn       *net.UDPConn
	mu         sync.RWMutex
	running    bool
	cache      map[string]*dnsCache
	cacheMu    sync.RWMutex
}

type dnsCache struct {
	ips       []net.IP
	expiresAt time.Time
}

// NewDNSServer 创建 DNS 服务器
// client: TunnoxMobileClient 实例
// listenAddr: 监听地址，如 "127.0.0.1:5353"
func NewDNSServer(client *TunnoxMobileClient, listenAddr string) *DNSServer {
	return &DNSServer{
		client:     client,
		listenAddr: listenAddr,
		cache:      make(map[string]*dnsCache),
	}
}

// Start 启动 DNS 服务器
// 返回错误信息，成功返回空字符串
func (s *DNSServer) Start() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return "DNS server already running"
	}

	addr, err := net.ResolveUDPAddr("udp", s.listenAddr)
	if err != nil {
		return fmt.Sprintf("failed to resolve address: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Sprintf("failed to listen: %v", err)
	}

	s.conn = conn
	s.running = true

	go s.serve()

	corelog.Infof("[DNSServer] started on %s", s.listenAddr)
	return ""
}

// Stop 停止 DNS 服务器
func (s *DNSServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	corelog.Infof("[DNSServer] stopped")
}

// IsRunning 检查是否正在运行
func (s *DNSServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetListenAddr 获取监听地址
func (s *DNSServer) GetListenAddr() string {
	return s.listenAddr
}

// serve 处理 DNS 请求
func (s *DNSServer) serve() {
	buf := make([]byte, 4096)

	for {
		s.mu.RLock()
		running := s.running
		conn := s.conn
		s.mu.RUnlock()

		if !running || conn == nil {
			return
		}

		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if !s.running {
				return
			}
			corelog.Warnf("[DNSServer] read error: %v", err)
			continue
		}

		// 处理 DNS 请求
		go s.handleQuery(buf[:n], addr)
	}
}

// handleQuery 处理单个 DNS 查询
func (s *DNSServer) handleQuery(query []byte, addr *net.UDPAddr) {
	if len(query) < 12 {
		return
	}

	// 解析域名和查询类型
	domain, qtype, err := parseDNSQuery(query)
	if err != nil {
		corelog.Warnf("[DNSServer] failed to parse query: %v", err)
		return
	}

	corelog.Debugf("[DNSServer] query: %s (type=%d) from %s", domain, qtype, addr.String())

	// 检查缓存
	cacheKey := fmt.Sprintf("%s:%d", domain, qtype)
	s.cacheMu.RLock()
	cached, exists := s.cache[cacheKey]
	s.cacheMu.RUnlock()

	var ips []net.IP
	if exists && time.Now().Before(cached.expiresAt) {
		ips = cached.ips
		corelog.Debugf("[DNSServer] cache hit: %s -> %v", domain, ips)
	} else {
		// 通过 tunnox 解析
		if s.client == nil || !s.client.IsConnected() {
			corelog.Warnf("[DNSServer] client not connected, falling back to system DNS")
			// 回退到系统 DNS
			addrs, err := net.LookupHost(domain)
			if err != nil {
				corelog.Warnf("[DNSServer] system DNS failed: %v", err)
				s.sendErrorResponse(query, addr)
				return
			}
			for _, a := range addrs {
				if ip := net.ParseIP(a); ip != nil {
					ips = append(ips, ip)
				}
			}
		} else {
			// 通过 tunnox 隧道解析
			result, err := s.client.ResolveDNS(domain, int64(qtype), -1)
			if err != nil {
				corelog.Warnf("[DNSServer] tunnel DNS failed: %v", err)
				s.sendErrorResponse(query, addr)
				return
			}

			// 解析结果
			for _, ipStr := range strings.Split(result, ",") {
				ipStr = strings.TrimSpace(ipStr)
				if ip := net.ParseIP(ipStr); ip != nil {
					ips = append(ips, ip)
				}
			}
		}

		// 缓存结果
		if len(ips) > 0 {
			s.cacheMu.Lock()
			s.cache[cacheKey] = &dnsCache{
				ips:       ips,
				expiresAt: time.Now().Add(5 * time.Minute),
			}
			s.cacheMu.Unlock()
		}
	}

	if len(ips) == 0 {
		corelog.Warnf("[DNSServer] no IPs resolved for %s", domain)
		s.sendErrorResponse(query, addr)
		return
	}

	corelog.Debugf("[DNSServer] resolved: %s -> %v", domain, ips)

	// 构造响应
	response, err := buildDNSResponse(query, ips, qtype)
	if err != nil {
		corelog.Warnf("[DNSServer] failed to build response: %v", err)
		return
	}

	// 发送响应
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()

	if conn != nil {
		conn.WriteToUDP(response, addr)
	}
}

// sendErrorResponse 发送 DNS 错误响应
func (s *DNSServer) sendErrorResponse(query []byte, addr *net.UDPAddr) {
	if len(query) < 12 {
		return
	}

	// 复制查询头部
	response := make([]byte, 12)
	copy(response, query[:12])

	// 设置标志: QR=1 (响应), RCODE=2 (SERVFAIL)
	response[2] = 0x81
	response[3] = 0x82

	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()

	if conn != nil {
		conn.WriteToUDP(response, addr)
	}
}

// parseDNSQuery 解析 DNS 查询包
func parseDNSQuery(query []byte) (string, uint16, error) {
	if len(query) < 12 {
		return "", 0, fmt.Errorf("query too short")
	}

	offset := 12
	var domain string

	for offset < len(query) {
		length := int(query[offset])
		if length == 0 {
			offset++
			break
		}
		if offset+1+length > len(query) {
			return "", 0, fmt.Errorf("invalid domain name")
		}
		if domain != "" {
			domain += "."
		}
		domain += string(query[offset+1 : offset+1+length])
		offset += 1 + length
	}

	if offset+2 > len(query) {
		return "", 0, fmt.Errorf("missing query type")
	}
	qtype := uint16(query[offset])<<8 | uint16(query[offset+1])

	return domain, qtype, nil
}

// buildDNSResponse 构造 DNS 响应包
func buildDNSResponse(query []byte, ips []net.IP, qtype uint16) ([]byte, error) {
	if len(query) < 12 {
		return nil, fmt.Errorf("invalid query")
	}

	response := make([]byte, 0, 512)
	response = append(response, query[:2]...) // Transaction ID
	response = append(response, 0x81, 0x80)   // Flags: QR=1, RD=1, RA=1

	// QDCOUNT = 1
	response = append(response, 0x00, 0x01)

	// 过滤 IP
	var validIPs []net.IP
	for _, ip := range ips {
		if qtype == 1 && ip.To4() != nil {
			validIPs = append(validIPs, ip)
		} else if qtype == 28 && ip.To4() == nil {
			validIPs = append(validIPs, ip)
		}
	}

	// ANCOUNT
	response = append(response, byte(len(validIPs)>>8), byte(len(validIPs)))

	// NSCOUNT = 0, ARCOUNT = 0
	response = append(response, 0x00, 0x00, 0x00, 0x00)

	// 复制问题部分
	questionEnd := 12
	for questionEnd < len(query) {
		if query[questionEnd] == 0 {
			questionEnd += 5
			break
		}
		questionEnd += int(query[questionEnd]) + 1
	}
	response = append(response, query[12:questionEnd]...)

	// 添加答案
	for _, ip := range validIPs {
		response = append(response, 0xc0, 0x0c) // 名称指针

		if qtype == 1 && ip.To4() != nil {
			response = append(response, 0x00, 0x01) // TYPE = A
			response = append(response, 0x00, 0x01) // CLASS = IN
			ttl := make([]byte, 4)
			binary.BigEndian.PutUint32(ttl, 300)
			response = append(response, ttl...)     // TTL
			response = append(response, 0x00, 0x04) // RDLENGTH
			response = append(response, ip.To4()...)
		} else if qtype == 28 && ip.To4() == nil {
			response = append(response, 0x00, 0x1c) // TYPE = AAAA
			response = append(response, 0x00, 0x01) // CLASS = IN
			ttl := make([]byte, 4)
			binary.BigEndian.PutUint32(ttl, 300)
			response = append(response, ttl...)     // TTL
			response = append(response, 0x00, 0x10) // RDLENGTH
			response = append(response, ip.To16()...)
		}
	}

	return response, nil
}

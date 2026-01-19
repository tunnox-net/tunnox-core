package socks5

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

const (
	udpRelayMaxPacketSize  = 65535
	udpSessionIdleTimeout  = 60 * time.Second
	maxUDPSessionsPerRelay = 128
)

type UDPTunnelCreator interface {
	CreateUDPTunnel(
		mappingID string,
		targetClientID int64,
		targetHost string,
		targetPort int,
		secretKey string,
	) (UDPTunnelConn, error)
}

// DNSQueryHandler DNS查询处理接口
// 通过控制通道发送DNS查询，避免UDP隧道的不稳定性
type DNSQueryHandler interface {
	// QueryDNS 发送DNS查询并返回响应
	// dnsServer: DNS服务器地址（如 "119.29.29.29:53"）
	// rawQuery: 原始DNS查询报文
	// 返回: 原始DNS响应报文
	QueryDNS(targetClientID int64, dnsServer string, rawQuery []byte) ([]byte, error)
}

type UDPTunnelConn interface {
	SendPacket(data []byte) error
	ReceivePacket() ([]byte, error)
	Close() error
}

type UDPRelayConfig struct {
	MappingID      string
	TargetClientID int64
	SecretKey      string
	BindAddr       string
}

type UDPRelay struct {
	*dispose.ServiceBase

	config        *UDPRelayConfig
	tcpConn       net.Conn
	udpConn       *net.UDPConn
	tunnelCreator UDPTunnelCreator
	dnsHandler    DNSQueryHandler // DNS查询处理器（通过控制通道）

	clientAddr   *net.UDPAddr
	clientAddrMu sync.RWMutex

	sessions   map[string]*udpSession
	sessionsMu sync.RWMutex
}

type udpSession struct {
	// lastActive 必须放在第一位，确保 64 位原子操作在 32 位 ARM 系统上对齐
	lastActive int64
	dstKey     string
	dstHost    string
	dstPort    int
	tunnel     UDPTunnelConn
	relay      *UDPRelay
	sendMu     sync.Mutex
}

func NewUDPRelay(
	ctx context.Context,
	tcpConn net.Conn,
	config *UDPRelayConfig,
	tunnelCreator UDPTunnelCreator,
) (*UDPRelay, error) {
	bindAddr := config.BindAddr
	if bindAddr == "" {
		bindAddr = "127.0.0.1:0"
	}

	udpAddr, err := net.ResolveUDPAddr("udp", bindAddr)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidParam, "invalid bind address")
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to bind UDP port")
	}

	relay := &UDPRelay{
		ServiceBase:   dispose.NewService("UDPRelay", ctx),
		config:        config,
		tcpConn:       tcpConn,
		udpConn:       udpConn,
		tunnelCreator: tunnelCreator,
		sessions:      make(map[string]*udpSession),
	}

	relay.AddCleanHandler(func() error {
		relay.closeAllSessions()
		return udpConn.Close()
	})

	go relay.watchTCPConnection()
	go relay.readLoop()
	go relay.cleanupLoop()

	return relay, nil
}

func (r *UDPRelay) GetBindAddr() *net.UDPAddr {
	return r.udpConn.LocalAddr().(*net.UDPAddr)
}

// SetDNSHandler 设置DNS查询处理器
func (r *UDPRelay) SetDNSHandler(handler DNSQueryHandler) {
	r.dnsHandler = handler
}

func (r *UDPRelay) watchTCPConnection() {
	buf := make([]byte, 1)
	for {
		if r.IsClosed() {
			return
		}

		r.tcpConn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, err := r.tcpConn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			corelog.Debugf("UDPRelay: TCP connection closed, terminating relay")
			r.Close()
			return
		}
	}
}

func (r *UDPRelay) readLoop() {
	buf := make([]byte, udpRelayMaxPacketSize)

	for {
		if r.IsClosed() {
			return
		}

		r.udpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, clientAddr, err := r.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if r.IsClosed() {
				return
			}
			corelog.Warnf("UDPRelay: read error: %v", err)
			continue
		}

		r.clientAddrMu.Lock()
		if r.clientAddr == nil {
			r.clientAddr = clientAddr
		} else if !r.clientAddr.IP.Equal(clientAddr.IP) {
			r.clientAddrMu.Unlock()
			corelog.Warnf("UDPRelay: dropping packet from unauthorized IP %s (expected %s)",
				clientAddr.IP, r.clientAddr.IP)
			continue
		}
		r.clientAddrMu.Unlock()

		dataCopy := make([]byte, n)
		copy(dataCopy, buf[:n])
		go r.handlePacket(dataCopy)
	}
}

// DefaultDNSServer 默认 DNS 服务器（当使用虚拟 DNS IP 时）
const DefaultDNSServer = "119.29.29.29"

func (r *UDPRelay) handlePacket(data []byte) {
	dstHost, dstPort, payload, err := r.parseUDPHeader(data)
	if err != nil {
		corelog.Warnf("UDPRelay: failed to parse UDP header: %v", err)
		return
	}

	// 检测DNS请求（端口53），通过控制通道处理
	if dstPort == 53 {
		if r.dnsHandler != nil {
			// 如果目标是虚拟 DNS IP，替换为真正的 DNS 服务器
			actualDNSHost := dstHost
			originalHost := dstHost // 保留原始地址用于构造响应
			if dstHost == VirtualDNSIP {
				actualDNSHost = DefaultDNSServer
				corelog.Infof("UDPRelay: DNS request to virtual DNS %s, using actual DNS server %s",
					dstHost, actualDNSHost)
			}
			corelog.Infof("UDPRelay: DNS request detected, routing via control channel to %s:%d", actualDNSHost, dstPort)
			r.handleDNSQuery(actualDNSHost, dstPort, payload, originalHost)
			return
		}
		corelog.Warnf("UDPRelay: DNS request to %s:%d but dnsHandler is nil, falling back to UDP tunnel", dstHost, dstPort)
	}

	session, err := r.getOrCreateSession(dstHost, dstPort)
	if err != nil {
		corelog.Warnf("UDPRelay: failed to create session for %s:%d: %v", dstHost, dstPort, err)
		return
	}

	session.sendMu.Lock()
	err = session.tunnel.SendPacket(payload)
	session.sendMu.Unlock()

	if err != nil {
		corelog.Warnf("UDPRelay: failed to send to tunnel: %v", err)
		r.removeSession(session.dstKey)
	}
}

// handleDNSQuery 通过控制通道处理DNS查询
// dstHost: 实际 DNS 服务器地址（用于查询）
// dstPort: DNS 端口
// payload: DNS 查询报文
// responseHost: 响应包的源地址（用于构造响应，可能是虚拟 DNS IP）
func (r *UDPRelay) handleDNSQuery(dstHost string, dstPort int, payload []byte, responseHost string) {
	dnsServer := fmt.Sprintf("%s:%d", dstHost, dstPort)

	corelog.Infof("UDPRelay: handleDNSQuery called, dnsServer=%s, targetClientID=%d, payloadLen=%d, responseHost=%s",
		dnsServer, r.config.TargetClientID, len(payload), responseHost)

	// 通过控制通道发送DNS查询
	response, err := r.dnsHandler.QueryDNS(r.config.TargetClientID, dnsServer, payload)
	if err != nil {
		corelog.Warnf("UDPRelay: DNS query failed for %s: %v", dnsServer, err)
		return
	}

	// 构造UDP响应包（使用原始目标地址作为响应源地址）
	packet := r.buildUDPHeader(responseHost, dstPort, response)

	// 发送给客户端
	r.clientAddrMu.RLock()
	clientAddr := r.clientAddr
	r.clientAddrMu.RUnlock()

	if clientAddr != nil {
		if _, err := r.udpConn.WriteToUDP(packet, clientAddr); err != nil {
			corelog.Warnf("UDPRelay: failed to send DNS response to client: %v", err)
		} else {
			corelog.Debugf("UDPRelay: DNS query via control channel success, server=%s, responseLen=%d", dnsServer, len(response))
		}
	}
}

func (r *UDPRelay) parseUDPHeader(data []byte) (string, int, []byte, error) {
	if len(data) < 10 {
		return "", 0, nil, coreerrors.New(coreerrors.CodeProtocolError, "packet too short")
	}

	if data[2] != 0x00 {
		return "", 0, nil, coreerrors.Newf(coreerrors.CodeProtocolError, "fragmentation not supported (FRAG=%d)", data[2])
	}

	addrType := data[3]
	var dstHost string
	var headerLen int

	switch addrType {
	case AddrIPv4:
		if len(data) < 10 {
			return "", 0, nil, coreerrors.New(coreerrors.CodeProtocolError, "packet too short for IPv4")
		}
		dstHost = net.IP(data[4:8]).String()
		headerLen = 10

	case AddrDomain:
		if len(data) < 5 {
			return "", 0, nil, coreerrors.New(coreerrors.CodeProtocolError, "packet too short for domain")
		}
		domainLen := int(data[4])
		if len(data) < 5+domainLen+2 {
			return "", 0, nil, coreerrors.New(coreerrors.CodeProtocolError, "packet too short for domain name")
		}
		dstHost = string(data[5 : 5+domainLen])
		headerLen = 5 + domainLen + 2

	case AddrIPv6:
		if len(data) < 22 {
			return "", 0, nil, coreerrors.New(coreerrors.CodeProtocolError, "packet too short for IPv6")
		}
		dstHost = net.IP(data[4:20]).String()
		headerLen = 22

	default:
		return "", 0, nil, coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported address type: %d", addrType)
	}

	portOffset := headerLen - 2
	dstPort := int(binary.BigEndian.Uint16(data[portOffset:headerLen]))
	payload := data[headerLen:]

	return dstHost, dstPort, payload, nil
}

func (r *UDPRelay) buildUDPHeader(dstHost string, dstPort int, payload []byte) []byte {
	ip := net.ParseIP(dstHost)
	var header []byte

	if ip4 := ip.To4(); ip4 != nil {
		header = make([]byte, 10+len(payload))
		header[0] = 0x00
		header[1] = 0x00
		header[2] = 0x00
		header[3] = AddrIPv4
		copy(header[4:8], ip4)
		binary.BigEndian.PutUint16(header[8:10], uint16(dstPort))
		copy(header[10:], payload)
	} else if ip16 := ip.To16(); ip16 != nil {
		header = make([]byte, 22+len(payload))
		header[0] = 0x00
		header[1] = 0x00
		header[2] = 0x00
		header[3] = AddrIPv6
		copy(header[4:20], ip16)
		binary.BigEndian.PutUint16(header[20:22], uint16(dstPort))
		copy(header[22:], payload)
	} else {
		domainBytes := []byte(dstHost)
		header = make([]byte, 5+len(domainBytes)+2+len(payload))
		header[0] = 0x00
		header[1] = 0x00
		header[2] = 0x00
		header[3] = AddrDomain
		header[4] = byte(len(domainBytes))
		copy(header[5:5+len(domainBytes)], domainBytes)
		binary.BigEndian.PutUint16(header[5+len(domainBytes):], uint16(dstPort))
		copy(header[5+len(domainBytes)+2:], payload)
	}

	return header
}

func (r *UDPRelay) getOrCreateSession(dstHost string, dstPort int) (*udpSession, error) {
	dstKey := fmt.Sprintf("%s:%d", dstHost, dstPort)

	r.sessionsMu.RLock()
	session, exists := r.sessions[dstKey]
	r.sessionsMu.RUnlock()

	if exists {
		atomic.StoreInt64(&session.lastActive, time.Now().UnixNano())
		return session, nil
	}

	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()

	if session, exists = r.sessions[dstKey]; exists {
		atomic.StoreInt64(&session.lastActive, time.Now().UnixNano())
		return session, nil
	}

	if len(r.sessions) >= maxUDPSessionsPerRelay {
		return nil, coreerrors.New(coreerrors.CodeResourceExhausted, "max UDP sessions reached")
	}

	tunnel, err := r.tunnelCreator.CreateUDPTunnel(
		r.config.MappingID,
		r.config.TargetClientID,
		dstHost,
		dstPort,
		r.config.SecretKey,
	)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to create UDP tunnel")
	}

	session = &udpSession{
		dstKey:     dstKey,
		dstHost:    dstHost,
		dstPort:    dstPort,
		tunnel:     tunnel,
		lastActive: time.Now().UnixNano(),
		relay:      r,
	}

	r.sessions[dstKey] = session

	go session.receiveLoop()

	corelog.Debugf("UDPRelay: created session for %s", dstKey)
	return session, nil
}

func (r *UDPRelay) removeSession(dstKey string) {
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()

	if session, exists := r.sessions[dstKey]; exists {
		session.tunnel.Close()
		delete(r.sessions, dstKey)
		corelog.Debugf("UDPRelay: removed session for %s", dstKey)
	}
}

func (r *UDPRelay) closeAllSessions() {
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()

	for dstKey, session := range r.sessions {
		session.tunnel.Close()
		delete(r.sessions, dstKey)
	}
}

func (r *UDPRelay) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Ctx().Done():
			return
		case <-ticker.C:
			r.cleanupIdleSessions()
		}
	}
}

func (r *UDPRelay) cleanupIdleSessions() {
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()

	nowNano := time.Now().UnixNano()
	timeoutNano := udpSessionIdleTimeout.Nanoseconds()

	for dstKey, session := range r.sessions {
		lastActive := atomic.LoadInt64(&session.lastActive)
		if nowNano-lastActive > timeoutNano {
			session.tunnel.Close()
			delete(r.sessions, dstKey)
			corelog.Debugf("UDPRelay: cleaned up idle session for %s", dstKey)
		}
	}
}

func (s *udpSession) receiveLoop() {
	for {
		if s.relay.IsClosed() {
			return
		}

		data, err := s.tunnel.ReceivePacket()
		if err != nil {
			if s.relay.IsClosed() {
				return
			}
			corelog.Warnf("UDPRelay: receive error from %s: %v", s.dstKey, err)
			s.relay.removeSession(s.dstKey)
			return
		}

		atomic.StoreInt64(&s.lastActive, time.Now().UnixNano())

		packet := s.relay.buildUDPHeader(s.dstHost, s.dstPort, data)

		s.relay.clientAddrMu.RLock()
		clientAddr := s.relay.clientAddr
		s.relay.clientAddrMu.RUnlock()

		if clientAddr != nil {
			if _, err := s.relay.udpConn.WriteToUDP(packet, clientAddr); err != nil {
				corelog.Warnf("UDPRelay: failed to send to client: %v", err)
			}
		}
	}
}

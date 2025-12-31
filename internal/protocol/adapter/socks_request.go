package adapter

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	coreerrors "tunnox-core/internal/core/errors"
)

// handleRequest 处理 SOCKS5 请求
func (s *SocksAdapter) handleRequest(conn net.Conn) (string, error) {
	// +----+-----+-------+------+----------+----------+
	// |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+

	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read request header failed")
	}

	version := buf[0]
	if version != socks5Version {
		return "", coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported SOCKS version: %d", version)
	}

	cmd := buf[1]
	// buf[2] 是保留字段
	addrType := buf[3]

	// 目前只支持 CONNECT 命令
	if cmd != socksCmdConnect {
		s.sendReply(conn, socksRepCommandNotSupported, "0.0.0.0", 0)
		return "", coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported command: %d", cmd)
	}

	// 解析目标地址
	var targetAddr string
	switch addrType {
	case socksAddrTypeIPv4:
		// IPv4 地址 (4 字节)
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read IPv4 address failed")
		}
		targetAddr = net.IP(addr).String()

	case socksAddrTypeDomain:
		// 域名 (1 字节长度 + 域名)
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read domain length failed")
		}
		domainLen := int(lenBuf[0])
		domain := make([]byte, domainLen)
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read domain failed")
		}
		targetAddr = string(domain)

	case socksAddrTypeIPv6:
		// IPv6 地址 (16 字节)
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read IPv6 address failed")
		}
		targetAddr = net.IP(addr).String()

	default:
		s.sendReply(conn, socksRepAddrTypeNotSupported, "0.0.0.0", 0)
		return "", coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported address type: %d", addrType)
	}

	// 读取端口 (2 字节，大端序)
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read port failed")
	}
	port := binary.BigEndian.Uint16(portBuf)

	return fmt.Sprintf("%s:%d", targetAddr, port), nil
}

// sendReply 发送 SOCKS5 响应
func (s *SocksAdapter) sendReply(conn net.Conn, rep byte, bindAddr string, bindPort uint16) error {
	// +----+-----+-------+------+----------+----------+
	// |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+

	ip := net.ParseIP(bindAddr)
	if ip == nil {
		ip = net.IPv4zero
	}

	reply := make([]byte, 0, 22)
	reply = append(reply, socks5Version, rep, 0x00) // VER, REP, RSV

	if ip4 := ip.To4(); ip4 != nil {
		reply = append(reply, socksAddrTypeIPv4)
		reply = append(reply, ip4...)
	} else {
		reply = append(reply, socksAddrTypeIPv6)
		reply = append(reply, ip.To16()...)
	}

	// 添加端口
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, bindPort)
	reply = append(reply, portBytes...)

	_, err := conn.Write(reply)
	return err
}

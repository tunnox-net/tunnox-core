package adapter

import (
	"io"
	"net"

	coreerrors "tunnox-core/internal/core/errors"
)

// handleHandshake 处理 SOCKS5 握手阶段
func (s *SocksAdapter) handleHandshake(conn net.Conn) error {
	// 读取客户端支持的认证方法
	// +----+----------+----------+
	// |VER | NMETHODS | METHODS  |
	// +----+----------+----------+
	// | 1  |    1     | 1 to 255 |
	// +----+----------+----------+

	buf := make([]byte, 257)
	n, err := io.ReadAtLeast(conn, buf, 2)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read handshake failed")
	}

	version := buf[0]
	if version != socks5Version {
		return coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported SOCKS version: %d", version)
	}

	nMethods := int(buf[1])
	if n < 2+nMethods {
		if _, err := io.ReadFull(conn, buf[n:2+nMethods]); err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read methods failed")
		}
	}

	methods := buf[2 : 2+nMethods]

	// 选择认证方法
	selectedMethod := socksAuthNoMatch
	if s.authEnabled {
		// 检查客户端是否支持用户名/密码认证
		for _, method := range methods {
			if method == socksAuthPassword {
				selectedMethod = socksAuthPassword
				break
			}
		}
	} else {
		// 检查客户端是否支持无认证
		for _, method := range methods {
			if method == socksAuthNone {
				selectedMethod = socksAuthNone
				break
			}
		}
	}

	// 发送选择的认证方法
	// +----+--------+
	// |VER | METHOD |
	// +----+--------+
	// | 1  |   1    |
	// +----+--------+
	if _, err := conn.Write([]byte{socks5Version, byte(selectedMethod)}); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeProtocolError, "write method selection failed")
	}

	if selectedMethod == socksAuthNoMatch {
		return coreerrors.New(coreerrors.CodeAuthFailed, "no acceptable authentication method")
	}

	// 如果需要认证，执行认证流程
	if selectedMethod == socksAuthPassword {
		if err := s.handlePasswordAuth(conn); err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeAuthFailed, "authentication failed")
		}
	}

	return nil
}

// handlePasswordAuth 处理用户名/密码认证
func (s *SocksAdapter) handlePasswordAuth(conn net.Conn) error {
	// +----+------+----------+------+----------+
	// |VER | ULEN |  UNAME   | PLEN |  PASSWD  |
	// +----+------+----------+------+----------+
	// | 1  |  1   | 1 to 255 |  1   | 1 to 255 |
	// +----+------+----------+------+----------+

	// 读取版本和用户名长度
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read auth header failed")
	}

	version := buf[0]
	if version != 0x01 {
		return coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported auth version: %d", version)
	}

	usernameLen := int(buf[1])

	// 读取用户名
	usernameBuf := make([]byte, usernameLen)
	if _, err := io.ReadFull(conn, usernameBuf); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read username failed")
	}
	username := string(usernameBuf)

	// 读取密码长度
	passwordLenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, passwordLenBuf); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read password length failed")
	}
	passwordLen := int(passwordLenBuf[0])

	// 读取密码
	passwordBuf := make([]byte, passwordLen)
	if _, err := io.ReadFull(conn, passwordBuf); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeProtocolError, "read password failed")
	}
	password := string(passwordBuf)

	// 验证凭据
	correctPassword, exists := s.credentials[username]
	success := exists && correctPassword == password

	// 发送认证响应
	// +----+--------+
	// |VER | STATUS |
	// +----+--------+
	// | 1  |   1    |
	// +----+--------+
	var status byte
	if success {
		status = 0x00 // 成功
	} else {
		status = 0x01 // 失败
	}

	if _, err := conn.Write([]byte{0x01, status}); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeProtocolError, "write auth response failed")
	}

	if !success {
		return coreerrors.New(coreerrors.CodeAuthFailed, "invalid credentials")
	}

	return nil
}

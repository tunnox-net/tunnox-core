// Package domainproxy 提供 HTTP 域名代理功能
package domainproxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"tunnox-core/internal/cloud/models"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
)

// writeHTTPRequestToTunnel 写入 HTTP 请求到隧道
func (m *DomainProxyModule) writeHTTPRequestToTunnel(
	tunnelConn httpservice.TunnelConnectionInterface,
	r *http.Request,
	mapping *models.PortMapping,
) error {
	// 1. 写入请求行
	requestLine := r.Method + " " + r.URL.RequestURI() + " HTTP/1.1\r\n"
	if _, err := tunnelConn.Write([]byte(requestLine)); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write request line")
	}

	// 2. 写入请求头
	// 添加/修改必要的头部
	headers := r.Header.Clone()

	// 设置 Host 头
	headers.Set("Host", mapping.TargetHost+":"+itoa(mapping.TargetPort))

	// 添加 X-Forwarded 头
	scheme := m.config.DefaultScheme
	if scheme == "" {
		scheme = "http"
	}
	headers.Set("X-Forwarded-For", r.RemoteAddr)
	headers.Set("X-Forwarded-Host", r.Host)
	headers.Set("X-Forwarded-Proto", scheme)

	// 移除 hop-by-hop 头
	headers.Del("Connection")
	headers.Del("Keep-Alive")
	headers.Del("Proxy-Authenticate")
	headers.Del("Proxy-Authorization")
	headers.Del("Te")
	headers.Del("Trailers")
	headers.Del("Transfer-Encoding")
	headers.Del("Upgrade")

	// 写入所有头部
	for key, values := range headers {
		for _, value := range values {
			headerLine := key + ": " + value + "\r\n"
			if _, err := tunnelConn.Write([]byte(headerLine)); err != nil {
				return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write header")
			}
		}
	}

	// 3. 写入空行（头部结束标记）
	if _, err := tunnelConn.Write([]byte("\r\n")); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write header end")
	}

	// 4. 复制请求体
	if r.Body != nil {
		defer r.Body.Close()

		buf := make([]byte, 32*1024) // 32KB buffer
		for {
			n, err := r.Body.Read(buf)
			if n > 0 {
				if _, writeErr := tunnelConn.Write(buf[:n]); writeErr != nil {
					return coreerrors.Wrap(writeErr, coreerrors.CodeNetworkError, "failed to write request body")
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read request body")
			}
		}
	}

	corelog.Debugf("DomainProxyModule: HTTP request written to tunnel successfully")
	return nil
}

// readHTTPResponseFromTunnel 从隧道读取 HTTP 响应
func (m *DomainProxyModule) readHTTPResponseFromTunnel(
	w http.ResponseWriter,
	tunnelConn httpservice.TunnelConnectionInterface,
) error {
	// 使用 bufio.Reader 读取 HTTP 响应
	reader := &tunnelReader{conn: tunnelConn}
	bufReader := io.Reader(reader)

	// 读取响应（使用简单的状态机解析）
	// 1. 读取状态行
	statusLine, err := m.readLine(bufReader)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read status line")
	}

	// 解析状态码
	var statusCode int
	if _, err := fmt.Sscanf(statusLine, "HTTP/1.%d %d", new(int), &statusCode); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInvalidRequest, "invalid status line")
	}

	// 2. 读取响应头
	headers := make(http.Header)
	for {
		line, err := m.readLine(bufReader)
		if err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read header")
		}

		// 空行表示头部结束
		if line == "" {
			break
		}

		// 解析头部
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if !isHopByHopHeader(key) {
				headers.Add(key, value)
			}
		}
	}

	// 3. 写入响应头到客户端
	for key, values := range headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(statusCode)

	// 4. 复制响应体
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := bufReader.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return coreerrors.Wrap(writeErr, coreerrors.CodeNetworkError, "failed to write response body")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read response body")
		}
	}

	corelog.Debugf("DomainProxyModule: HTTP response read from tunnel successfully")
	return nil
}

// readLine 从 reader 读取一行（以 \r\n 结尾）
func (m *DomainProxyModule) readLine(reader io.Reader) (string, error) {
	var line []byte
	buf := make([]byte, 1)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}

		line = append(line, buf[0])

		// 检查是否为 \r\n 结尾
		if len(line) >= 2 && line[len(line)-2] == '\r' && line[len(line)-1] == '\n' {
			return string(line[:len(line)-2]), nil
		}
	}
}

// tunnelReader 包装 TunnelConnectionInterface 为 io.Reader
type tunnelReader struct {
	conn httpservice.TunnelConnectionInterface
}

func (r *tunnelReader) Read(p []byte) (int, error) {
	return r.conn.Read(p)
}

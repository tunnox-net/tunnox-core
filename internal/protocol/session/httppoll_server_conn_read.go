package session

import (
corelog "tunnox-core/internal/core/log"
	"encoding/base64"
	"fmt"
	"io"

)

// Read 实现 io.Reader（从 HTTP POST 读取数据）
func (c *ServerHTTPLongPollingConn) Read(p []byte) (int, error) {
	c.closeMu.RLock()
	closed := c.closed
	c.closeMu.RUnlock()

	if closed {
		return 0, io.EOF
	}

	c.readBufMu.Lock()
	defer c.readBufMu.Unlock()

	// 先检查缓冲区是否有数据
	// ✅ 在流模式下，限制每次最多读取一个 MySQL 协议包的大小，避免跨包读取
	// MySQL 协议包结构：前3字节是包长度（小端序），第4字节是序列号
	c.streamMu.RLock()
	streamMode := c.streamMode
	c.streamMu.RUnlock()

	connID := c.GetConnectionID()
	corelog.Infof("HTTP long polling: [READ] entry, streamMode=%v, readBuffer len=%d, requested=%d, clientID=%d, connID=%s",
		streamMode, len(c.readBuffer), len(p), c.clientID, connID)

	if streamMode && len(c.readBuffer) >= 4 {
		// 解析 MySQL 协议包长度（前3字节，小端序）
		packetLength := int(c.readBuffer[0]) | int(c.readBuffer[1])<<8 | int(c.readBuffer[2])<<16
		// MySQL 协议包总大小 = 3字节长度 + 1字节序列号 + 包体长度
		packetSize := 4 + packetLength

		// 验证包长度是否合理（防止解析错误）
		// MySQL 协议包最大为 16MB (2^24 - 1)，但实际使用中通常不会超过 64KB
		if packetLength > 0 && packetLength <= 16*1024*1024 {
			// 如果缓冲区有完整包，只读取一个包
			if len(c.readBuffer) >= packetSize {
				readSize := packetSize
				if readSize > len(p) {
					readSize = len(p)
				}
				n := copy(p[:readSize], c.readBuffer[:readSize])
				c.readBuffer = c.readBuffer[n:]
				corelog.Infof("HTTP long polling: [READ] read %d bytes (MySQL packet, length=%d, remaining: %d), clientID=%d, connID=%s",
					n, packetLength, len(c.readBuffer), c.clientID, c.GetConnectionID())
				return n, nil
			}
			// ✅ 如果缓冲区没有完整包，尝试从 base64PushDataChan 接收更多数据（非阻塞）
			// 如果 channel 为空，立即返回部分数据，避免阻塞导致超时
			corelog.Debugf("HTTP long polling: [READ] incomplete MySQL packet (need %d bytes, have %d), trying to get more data, clientID=%d, connID=%s",
				packetSize, len(c.readBuffer), c.clientID, c.GetConnectionID())
			// 尝试从 base64PushDataChan 接收更多数据（非阻塞）
			select {
			case base64Data, ok := <-c.base64PushDataChan:
				if !ok {
					// channel 已关闭，返回 EOF
					return 0, io.EOF
				}
				// Base64 解码
				data, err := base64.StdEncoding.DecodeString(base64Data)
				if err != nil {
					corelog.Errorf("HTTP long polling: [READ] failed to decode Base64 data: %v, clientID=%d", err, c.clientID)
					return 0, fmt.Errorf("failed to decode Base64 data: %w", err)
				}
				// 追加到 readBuffer
				c.readBuffer = append(c.readBuffer, data...)
				corelog.Debugf("HTTP long polling: [READ] received %d bytes from channel, buffer size now: %d, clientID=%d",
					len(data), len(c.readBuffer), c.clientID)
				// 重新检查是否有完整包
				if len(c.readBuffer) >= packetSize {
					readSize := packetSize
					if readSize > len(p) {
						readSize = len(p)
					}
					n := copy(p[:readSize], c.readBuffer[:readSize])
					c.readBuffer = c.readBuffer[n:]
					corelog.Infof("HTTP long polling: [READ] read %d bytes (MySQL packet, length=%d, remaining: %d), clientID=%d, connID=%s",
						n, packetLength, len(c.readBuffer), c.clientID, c.GetConnectionID())
					return n, nil
				}
				// 仍然没有完整包，返回部分数据（避免阻塞）
				readSize := len(c.readBuffer)
				if readSize > len(p) {
					readSize = len(p)
				}
				n := copy(p[:readSize], c.readBuffer[:readSize])
				c.readBuffer = c.readBuffer[n:]
				corelog.Debugf("HTTP long polling: [READ] read %d bytes (partial packet, remaining: %d), clientID=%d",
					n, len(c.readBuffer), c.clientID)
				return n, nil
			default:
				// channel 为空，立即返回部分数据，避免阻塞
				readSize := len(c.readBuffer)
				if readSize > len(p) {
					readSize = len(p)
				}
				n := copy(p[:readSize], c.readBuffer[:readSize])
				c.readBuffer = c.readBuffer[n:]
				corelog.Debugf("HTTP long polling: [READ] read %d bytes (partial packet, channel empty, remaining: %d), clientID=%d",
					n, len(c.readBuffer), c.clientID)
				return n, nil
			}
		}
	}

	// 如果缓冲区数据不足4字节，或者无法解析包长度，按原逻辑读取
	if len(c.readBuffer) > 0 {
		readSize := len(c.readBuffer)
		if readSize > len(p) {
			readSize = len(p)
		}
		// ✅ 限制最大读取大小为 64KB，避免将多个 HTTP push 请求合并成一个大的读取
		maxReadSize := 64 * 1024
		if readSize > maxReadSize {
			readSize = maxReadSize
		}
		n := copy(p[:readSize], c.readBuffer[:readSize])
		c.readBuffer = c.readBuffer[n:]
		corelog.Debugf("HTTP long polling: [READ] read %d bytes from buffer (remaining: %d), clientID=%d",
			n, len(c.readBuffer), c.clientID)
		return n, nil
	}

	// 缓冲区为空，从 base64PushDataChan 接收 Base64 数据并解码
	corelog.Infof("HTTP long polling: [READ] waiting for Base64 data from base64PushDataChan, clientID=%d, connID=%s, channel len=%d, cap=%d",
		c.clientID, c.GetConnectionID(), len(c.base64PushDataChan), cap(c.base64PushDataChan))
	// 注意：这里不使用超时，因为 ReadPacket 会持续调用 Read，直到读取完整数据包
	// 如果 channel 为空，应该阻塞等待，而不是返回 EOF
	select {
	case <-c.Ctx().Done():
		corelog.Infof("HTTP long polling: [READ] context canceled, clientID=%d, connID=%s", c.clientID, c.GetConnectionID())
		return 0, c.Ctx().Err()
	case base64Data, ok := <-c.base64PushDataChan:
		if !ok {
			corelog.Debugf("HTTP long polling: [READ] base64PushDataChan closed, clientID=%d", c.clientID)
			return 0, io.EOF
		}

		// Base64 解码
		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			corelog.Errorf("HTTP long polling: [READ] failed to decode Base64 data: %v, clientID=%d", err, c.clientID)
			return 0, fmt.Errorf("failed to decode Base64 data: %w", err)
		}

		corelog.Infof("HTTP long polling: [READ] decoded %d bytes from Base64 data, clientID=%d, connID=%s",
			len(data), c.clientID, c.GetConnectionID())

		// 追加到 readBuffer
		c.readBuffer = append(c.readBuffer, data...)

		// 从 readBuffer 读取
		n := copy(p, c.readBuffer)
		c.readBuffer = c.readBuffer[n:]
		firstByte := byte(0)
		if len(c.readBuffer) > 0 {
			firstByte = c.readBuffer[0]
		}
		corelog.Infof("HTTP long polling: [READ] read %d bytes (remaining in buffer: %d), clientID=%d, connID=%s, firstByte=0x%02x",
			n, len(c.readBuffer), c.clientID, c.GetConnectionID(), firstByte)
		return n, nil
	}
}


package client

import (
	"encoding/binary"
	"io"
	"runtime"
	"strings"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/packet"
)

func (c *HTTPLongPollingConn) Write(p []byte) (int, error) {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()

	if closed {
		return 0, io.ErrClosedPipe
	}

	// 检查是否是流模式
	c.streamMu.RLock()
	streamMode := c.streamMode
	c.streamMu.RUnlock()

	// 流模式：直接发送数据，不等待完整包
	// 注意：MySQL等协议需要保持数据包完整性，不能随意分片
	// 因此，即使数据很大，也要保持完整发送，让协议层自己处理
	if streamMode {
		firstByte := byte(0)
		if len(p) > 0 {
			firstByte = p[0]
		}

		// 直接发送数据，保持协议包完整性
		// 如果数据过大，HTTP层会处理（如超时、分块传输等）
		corelog.Debugf("HTTP long polling: [Write] stream mode: sending %d bytes, firstByte=0x%02x, mappingID=%s",
			len(p), firstByte, c.mappingID)

		// 直接发送数据
		if err := c.sendData(p); err != nil {
			corelog.Errorf("HTTP long polling: [Write] stream mode: failed to send data: %v", err)
			return 0, err
		}
		return len(p), nil
	}

	// 非流模式：验证写入的数据不是 Base64 字符串（防止 Base64 数据被错误写入）
	if len(p) > 0 {
		isBase64Char := func(b byte) bool {
			return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') ||
				(b >= '0' && b <= '9') || b == '+' || b == '/' || b == '='
		}
		base64Count := 0
		for i := 0; i < len(p) && i < 10; i++ {
			if isBase64Char(p[i]) {
				base64Count++
			}
		}
		if base64Count >= 8 {
			previewLen := 20
			if len(p) < previewLen {
				previewLen = len(p)
			}
			corelog.Errorf("HTTP long polling: Write called with Base64-like data (first %d bytes are Base64 chars), possible error", base64Count)
			corelog.Errorf("HTTP long polling: Write data preview (first %d bytes): %q, hex: %x", previewLen, string(p[:previewLen]), p[:previewLen])
		}
	}

	// 将数据写入缓冲区
	c.writeBufMu.Lock()
	n, err := c.writeBuffer.Write(p)
	bufLen := c.writeBuffer.Len()
	c.writeBufMu.Unlock()

	firstByte := byte(0)
	if len(p) > 0 {
		firstByte = p[0]
	}

	// 如果是心跳包类型（0x43 = 0x03 | 0x40），添加更详细的日志
	if firstByte == 0x43 && len(p) == 1 {
		corelog.Debugf("HTTP long polling: Write called with heartbeat packet type (0x43), len=%d, bufferLen=%d", len(p), bufLen)
		// 打印调用栈（仅前 5 层）
		corelog.Debugf("HTTP long polling: Write call stack (first 5 frames):")
		for i := 1; i <= 5; i++ {
			pc, file, line, ok := runtime.Caller(i)
			if ok {
				fn := runtime.FuncForPC(pc)
				if fn != nil {
					// 只显示文件名和函数名，不显示完整路径
					fileName := file
					if idx := strings.LastIndex(file, "/"); idx >= 0 {
						fileName = file[idx+1:]
					}
					funcName := fn.Name()
					if idx := strings.LastIndex(funcName, "."); idx >= 0 {
						funcName = funcName[idx+1:]
					}
					corelog.Debugf("  [%d] %s:%d %s", i, fileName, line, funcName)
				}
			}
		}
	} else {
		corelog.Debugf("HTTP long polling: Write called, len=%d, bufferLen=%d, firstByte=0x%02x", len(p), bufLen, firstByte)
	}

	if err != nil {
		return 0, err
	}

	// 触发刷新检查（非阻塞）
	select {
	case c.writeFlush <- struct{}{}:
	default:
	}

	return n, nil
}

// writeFlushLoop 写入刷新循环（检查完整包并发送）
func (c *HTTPLongPollingConn) writeFlushLoop() {
	corelog.Infof("HTTP long polling: writeFlushLoop started")
	ticker := time.NewTicker(50 * time.Millisecond) // 每50ms检查一次
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx().Done():
			return
		case <-ticker.C:
			// 定期检查缓冲区
		case <-c.writeFlush:
			// 收到刷新信号，立即检查
			corelog.Infof("HTTP long polling: writeFlushLoop received flush signal")
		}

		// 检查缓冲区是否有完整包
		c.writeBufMu.Lock()
		bufLen := c.writeBuffer.Len()

		// ✅ 特殊处理：心跳包只有 1 字节（包类型），没有包体大小和包体
		// 注意：心跳包应该只通过控制连接发送，不应该通过隧道连接
		// 如果缓冲区有数据，先检查是否是心跳包
		if bufLen >= 1 {
			bufData := c.writeBuffer.Bytes()
			packetType := packet.Type(bufData[0])
			// 检查是否是心跳包（忽略压缩/加密标志）
			if packetType.IsHeartbeat() {
				// ✅ 心跳包应该只通过控制连接发送，不应该通过隧道连接
				// 如果是隧道连接（mappingID 不为空），心跳包不应该出现在这里
				if c.mappingID != "" {
					corelog.Errorf("HTTP long polling: writeFlushLoop detected heartbeat packet in tunnel connection (mappingID=%s), dropping it", c.mappingID)
					// 丢弃心跳包，清空缓冲区
					c.writeBuffer.Reset()
					c.writeBufMu.Unlock()
					continue
				}
				// 心跳包只有 1 字节，直接发送（仅控制连接）
				data := make([]byte, 1)
				copy(data, bufData[:1])
				c.writeBuffer.Next(1)
				c.writeBufMu.Unlock()

				corelog.Debugf("HTTP long polling: writeFlushLoop sending heartbeat packet (0x%02x) on control connection", data[0])
				if err := c.sendData(data); err != nil {
					corelog.Errorf("HTTP long polling: failed to send heartbeat packet: %v", err)
				}
				continue
			}
		}

		if bufLen >= 5 {
			// 至少有一个包类型（1字节）+ 包体大小（4字节）
			bufData := c.writeBuffer.Bytes()

			// 解析包体大小（大端序，从第2到第5字节，即索引1-4）
			// 注意：必须确保有足够的字节才能解析
			if len(bufData) < 5 {
				c.writeBufMu.Unlock()
				continue
			}

			// 调试：打印前5字节的十六进制值
			corelog.Debugf("HTTP long polling: writeFlushLoop buffer first 5 bytes: %02x %02x %02x %02x %02x",
				bufData[0], bufData[1], bufData[2], bufData[3], bufData[4])

			// 检查包类型是否有效（应该是 0x00-0xFF 范围内的值，但通常不会超过 0x3F + 标志位）
			packetType := bufData[0]

			// 检查是否是有效的包类型（排除明显无效的值）
			// 包类型的基础值应该在 0x00-0x3F 范围内，加上标志位（Compressed=0x40, Encrypted=0x80）
			// 所以有效范围是 0x00-0xFF，但排除一些明显无效的值
			basePacketType := packetType & 0x3F
			if basePacketType > 0x3F {
				// 基础包类型无效
				corelog.Errorf("HTTP long polling: invalid base packet type 0x%02x, resetting buffer", basePacketType)
				c.writeBuffer.Reset()
				c.writeBufMu.Unlock()
				continue
			}

			bodySize := binary.BigEndian.Uint32(bufData[1:5])

			// 计算完整包大小：1字节类型 + 4字节大小 + bodySize
			packetSize := 5 + int(bodySize)

			// 验证包体大小是否合理（防止解析错误导致无限等待）
			// 正常的数据包体大小应该在 0-10MB 范围内
			if bodySize > 10*1024*1024 { // 10MB 上限
				corelog.Errorf("HTTP long polling: invalid bodySize=%d (too large), packetType=0x%02x, first 5 bytes: %02x %02x %02x %02x %02x, resetting buffer",
					bodySize, packetType, bufData[0], bufData[1], bufData[2], bufData[3], bufData[4])
				c.writeBuffer.Reset()
				c.writeBufMu.Unlock()
				continue
			}

			// 额外检查：如果前5字节都是相同的值（如 43 43 43 43 43），可能是数据损坏
			if bufData[0] == bufData[1] && bufData[1] == bufData[2] && bufData[2] == bufData[3] && bufData[3] == bufData[4] {
				corelog.Errorf("HTTP long polling: suspicious data pattern (all bytes same: 0x%02x), resetting buffer", bufData[0])
				c.writeBuffer.Reset()
				c.writeBufMu.Unlock()
				continue
			}

			// 检查是否是 Base64 字符（A-Z, a-z, 0-9, +, /, =）
			// 如果前5字节都是 Base64 字符，可能是 Base64 字符串的字节被错误写入
			isBase64Char := func(b byte) bool {
				return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') ||
					(b >= '0' && b <= '9') || b == '+' || b == '/' || b == '='
			}
			if isBase64Char(bufData[0]) && isBase64Char(bufData[1]) &&
				isBase64Char(bufData[2]) && isBase64Char(bufData[3]) && isBase64Char(bufData[4]) {
				// 检查是否连续多个字节都是 Base64 字符（可能是 Base64 字符串）
				base64Count := 0
				for i := 0; i < len(bufData) && i < 20; i++ {
					if isBase64Char(bufData[i]) {
						base64Count++
					} else {
						break
					}
				}
				if base64Count >= 10 {
					corelog.Errorf("HTTP long polling: detected Base64 string in writeBuffer (first %d bytes are Base64 chars), resetting buffer", base64Count)
					c.writeBuffer.Reset()
					c.writeBufMu.Unlock()
					continue
				}
			}

			corelog.Debugf("HTTP long polling: writeFlushLoop checking buffer, bufLen=%d, bodySize=%d, packetSize=%d", bufLen, bodySize, packetSize)

			if bufLen >= packetSize {
				// 有完整包，提取并发送
				data := make([]byte, packetSize)
				copy(data, bufData[:packetSize])
				c.writeBuffer.Next(packetSize)
				c.writeBufMu.Unlock()

				corelog.Infof("HTTP long polling: writeFlushLoop sending complete packet, size=%d", packetSize)
				// 发送数据
				if err := c.sendData(data); err != nil {
					corelog.Errorf("HTTP long polling: failed to send buffered data: %v", err)
				}
				continue
			}
		}
		c.writeBufMu.Unlock()
	}
}

package client

import (
corelog "tunnox-core/internal/core/log"
	"encoding/base64"
	"fmt"
	"io"

	"tunnox-core/internal/packet"
)

func (c *HTTPLongPollingConn) Unread(data []byte) {
	if len(data) == 0 {
		return
	}
	c.readBufMu.Lock()
	defer c.readBufMu.Unlock()
	// 将数据放回 readBuffer 的开头
	c.readBuffer = append(data, c.readBuffer...)
	corelog.Infof("HTTP long polling: [Unread] restored %d bytes to readBuffer (total: %d), mappingID=%s",
		len(data), len(c.readBuffer), c.mappingID)
}

// Read 实现 io.Reader 接口（从字节流缓冲区读取数据）
// 按照 Base64 适配层设计：Base64 解码后的数据追加到 readBuffer，Read 从 readBuffer 按字节读取
// 流模式下：直接返回原始数据，不解析数据包格式
func (c *HTTPLongPollingConn) Read(p []byte) (int, error) {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()

	if closed {
		return 0, io.EOF
	}

	// 检查流模式
	c.streamMu.RLock()
	streamMode := c.streamMode
	c.streamMu.RUnlock()

	c.readBufMu.Lock()
	// 先检查缓冲区是否有数据
	if len(c.readBuffer) > 0 {
		n := copy(p, c.readBuffer)
		// 保存读取的数据到 peekBuffer（用于 Unread）
		c.peekBufMu.Lock()
		c.peekBuffer = append(c.peekBuffer, c.readBuffer[:n]...)
		c.peekBufMu.Unlock()
		c.readBuffer = c.readBuffer[n:]
		c.readBufMu.Unlock()
		return n, nil
	}
	c.readBufMu.Unlock()

	// readBuffer 为空，从 base64DataChan 接收 Base64 数据并解码
	select {
	case <-c.Ctx().Done():
		return 0, c.Ctx().Err()
	case base64Data, ok := <-c.base64DataChan:
		if !ok {
			return 0, io.EOF
		}

		// Base64 解码
		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			corelog.Errorf("HTTP long polling: failed to decode Base64 data (len=%d): %v", len(base64Data), err)
			// 打印前20个字符用于调试
			preview := base64Data
			if len(preview) > 20 {
				preview = preview[:20]
			}
			corelog.Errorf("HTTP long polling: Base64 data preview: %s", preview)
			return 0, fmt.Errorf("failed to decode Base64 data: %w", err)
		}

		// 验证解码后的数据不是 Base64 字符串（防止循环编码）
		// 注意：对于流模式，数据可能是任意二进制数据，包括Base64字符
		// 所以这个检查应该更宽松，或者只在非流模式下进行
		if len(data) > 0 && !streamMode {
			isBase64Char := func(b byte) bool {
				return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') ||
					(b >= '0' && b <= '9') || b == '+' || b == '/' || b == '='
			}
			base64Count := 0
			for i := 0; i < len(data) && i < 20; i++ {
				if isBase64Char(data[i]) {
					base64Count++
				}
			}
			// 提高阈值，避免误判（MySQL等协议的数据可能包含Base64字符）
			if base64Count >= 15 {
				corelog.Warnf("HTTP long polling: decoded data appears to be Base64 string (first %d bytes are Base64 chars), possible double encoding", base64Count)
				// 不返回错误，只记录警告，因为可能是误判
			}
		}

		// 追加到 readBuffer
		c.readBufMu.Lock()
		oldBufferLen := len(c.readBuffer)
		c.readBuffer = append(c.readBuffer, data...)
		newBufferLen := len(c.readBuffer)
		corelog.Debugf("HTTP long polling: [Read] appended %d bytes to readBuffer (old len=%d, new len=%d), mappingID=%s",
			len(data), oldBufferLen, newBufferLen, c.mappingID)

		// 只有指令通道（control）才需要过滤心跳包
		// 数据通道（data）不应该有心跳包，数据流中的 0x03 字节是正常数据，不应该被过滤
		if !streamMode && c.connType == "control" && len(c.readBuffer) > 0 {
			// 检查 readBuffer 中是否有心跳包，如果有则过滤掉
			// 注意：只在非流模式的指令通道中过滤，避免误过滤数据流中的正常数据
			filtered := make([]byte, 0, len(c.readBuffer))
			for i := 0; i < len(c.readBuffer); i++ {
				// 检查是否是心跳包（0x03 或 0x43）
				packetType := packet.Type(c.readBuffer[i])
				if packetType.IsHeartbeat() {
					corelog.Debugf("HTTP long polling: [Read] control channel: filtering heartbeat packet (0x%02x) at index %d",
						c.readBuffer[i], i)
					continue // 跳过心跳包
				}
				filtered = append(filtered, c.readBuffer[i])
			}
			if len(filtered) != len(c.readBuffer) {
				corelog.Debugf("HTTP long polling: [Read] filtered %d bytes from readBuffer (before=%d, after=%d)",
					len(c.readBuffer)-len(filtered), len(c.readBuffer), len(filtered))
			}
			c.readBuffer = filtered
		}

		// 从 readBuffer 读取
		n := copy(p, c.readBuffer)
		c.readBuffer = c.readBuffer[n:]
		c.readBufMu.Unlock()
		corelog.Debugf("HTTP long polling: [Read] copied %d bytes from readBuffer, mappingID=%s",
			n, c.mappingID)

		// 流模式下，直接返回数据，不验证 Base64 格式（因为已经是原始数据）
		if !streamMode {
			// 非流模式：验证读取的数据不是 Base64 字符串（防止 Base64 数据被错误返回）
			if n > 0 && len(p) > 0 {
				isBase64Char := func(b byte) bool {
					return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') ||
						(b >= '0' && b <= '9') || b == '+' || b == '/' || b == '='
				}
				base64Count := 0
				for i := 0; i < n && i < 10; i++ {
					if isBase64Char(p[i]) {
						base64Count++
					}
				}
				if base64Count >= 8 {
					previewLen := 20
					if n < previewLen {
						previewLen = n
					}
					corelog.Errorf("HTTP long polling: Read returned Base64-like data (first %d bytes are Base64 chars), possible error", base64Count)
					corelog.Errorf("HTTP long polling: Read data preview (first %d bytes): %q, hex: %x", previewLen, string(p[:previewLen]), p[:previewLen])
				}
			}
		}

		return n, nil
	}
}

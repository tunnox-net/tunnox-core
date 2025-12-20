package session

import (
	"encoding/binary"
	"io"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/packet"
)

// Write 实现 io.Writer（通过 HTTP GET 响应发送数据）
// 注意：StreamProcessor.WritePacket() 会多次调用 Write()（包类型、包体大小、包体）
// 我们需要缓冲这些数据，直到收到完整的包后再发送
func (c *ServerHTTPLongPollingConn) Write(p []byte) (int, error) {
	c.closeMu.RLock()
	closed := c.closed
	c.closeMu.RUnlock()

	if closed {
		corelog.Warnf("HTTP long polling: [WRITE] connection closed, clientID=%d", c.GetClientID())
		return 0, io.ErrClosedPipe
	}

	// 检查是否已切换到流模式
	c.streamMu.RLock()
	streamMode := c.streamMode
	c.streamMu.RUnlock()

	// 流模式下，直接转发数据，不缓冲（但过滤心跳包）
	if streamMode {
		clientID := c.GetClientID()
		mappingID := c.GetMappingID()
		// ✅ 过滤心跳包：心跳包应该只通过控制连接发送，不应该通过隧道连接
		// 如果数据是1字节且是心跳包类型，且是隧道连接（mappingID 不为空），则丢弃
		if len(p) == 1 {
			packetType := packet.Type(p[0])
			if packetType.IsHeartbeat() {
				// 如果是隧道连接，心跳包不应该出现在这里
				if mappingID != "" {
					corelog.Errorf("HTTP long polling: [WRITE] stream mode: detected heartbeat packet in tunnel connection (mappingID=%s), dropping it, clientID=%d",
						mappingID, clientID)
					return len(p), nil // 丢弃心跳包，但返回成功
				}
				// 控制连接的心跳包，正常处理
				corelog.Debugf("HTTP long polling: [WRITE] stream mode: heartbeat packet on control connection (0x%02x), clientID=%d",
					p[0], clientID)
			}
		}

		firstByte := byte(0)
		if len(p) > 0 {
			firstByte = p[0]
		}
		corelog.Infof("HTTP long polling: [WRITE] stream mode: pushing %d bytes directly to priority queue, clientID=%d, mappingID=%s, firstByte=0x%02x",
			len(p), clientID, mappingID, firstByte)
		c.pollDataQueue.Push(p)

		// 立即通知 pollDataScheduler 有数据可用（非阻塞）
		select {
		case c.pollWaitChan <- struct{}{}:
		default:
		}
		return len(p), nil
	}

	// 非流模式：将数据写入缓冲区
	c.writeBufMu.Lock()
	n, err := c.writeBuffer.Write(p)
	bufLen := c.writeBuffer.Len()
	c.writeBufMu.Unlock()

	corelog.Debugf("HTTP long polling: [WRITE] writing %d bytes to buffer, bufferLen=%d, clientID=%d",
		len(p), bufLen, c.GetClientID())

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
func (c *ServerHTTPLongPollingConn) writeFlushLoop() {
	clientID := c.GetClientID()
	corelog.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] started, clientID=%d", clientID)
	ticker := time.NewTicker(50 * time.Millisecond) // 每50ms检查一次
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx().Done():
			corelog.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] context canceled, clientID=%d", c.GetClientID())
			return
		case <-ticker.C:
			// 定期检查缓冲区
		case <-c.writeFlush:
			// 收到刷新信号，立即检查
			corelog.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] received flush signal, clientID=%d", c.GetClientID())
		}

		// 检查是否已切换到流模式
		c.streamMu.RLock()
		streamMode := c.streamMode
		c.streamMu.RUnlock()

		// 检查缓冲区是否有完整包
		c.writeBufMu.Lock()
		bufLen := c.writeBuffer.Len()

		// 如果已切换到流模式，直接转发原始数据，不再解析数据包格式
		if streamMode && bufLen > 0 {
			data := make([]byte, bufLen)
			copy(data, c.writeBuffer.Bytes())
			c.writeBuffer.Reset()
			c.writeBufMu.Unlock()

			corelog.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] stream mode: pushing %d bytes directly to priority queue, clientID=%d, mappingID=%s",
				len(data), c.GetClientID(), c.GetMappingID())
			c.pollDataQueue.Push(data)

			// 立即通知 pollDataScheduler 有数据可用（非阻塞）
			select {
			case c.pollWaitChan <- struct{}{}:
			default:
			}
			continue
		}

		// 特殊处理：心跳包只有 1 字节（包类型），没有包体大小和包体
		// 如果缓冲区有数据，先检查是否是心跳包
		if bufLen >= 1 {
			bufData := c.writeBuffer.Bytes()
			packetType := packet.Type(bufData[0])
			// 检查是否是心跳包（忽略压缩/加密标志）
			if packetType.IsHeartbeat() {
				// 心跳包只有 1 字节，直接发送
				data := make([]byte, 1)
				copy(data, bufData[:1])
				c.writeBuffer.Next(1)
				c.writeBufMu.Unlock()

				corelog.Infof("HTTP long polling: [WRITE_FLUSH_LOOP] pushing heartbeat packet (0x%02x) to priority queue, clientID=%d", data[0], c.GetClientID())
				// 心跳包推入优先级队列（会被合并/丢弃多余的心跳包）
				c.pollDataQueue.Push(data)
				continue
			}
		}

		if bufLen >= 5 {
			// 至少有一个包类型（1字节）+ 包体大小（4字节）
			bufData := c.writeBuffer.Bytes()

			// 解析包体大小（大端序，从第2到第5字节，即索引1-4）
			if len(bufData) < 5 {
				c.writeBufMu.Unlock()
				continue
			}

			bodySize := binary.BigEndian.Uint32(bufData[1:5])

			// 计算完整包大小：1字节类型 + 4字节大小 + bodySize
			packetSize := 5 + int(bodySize)

			// 验证包体大小是否合理（防止解析错误导致无限等待）
			if bodySize > 10*1024*1024 { // 10MB 上限
				corelog.Errorf("HTTP long polling: [WRITE_FLUSH_LOOP] invalid bodySize=%d (too large), resetting buffer, clientID=%d",
					bodySize, c.GetClientID())
				c.writeBuffer.Reset()
				c.writeBufMu.Unlock()
				continue
			}

			corelog.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] checking buffer, bufLen=%d, bodySize=%d, packetSize=%d, clientID=%d",
				bufLen, bodySize, packetSize, c.GetClientID())

			if bufLen >= packetSize {
				// 有完整包，提取并发送
				data := make([]byte, packetSize)
				copy(data, bufData[:packetSize])
				c.writeBuffer.Next(packetSize)
				c.writeBufMu.Unlock()

				// 发送完整数据包到优先级队列（优先级队列会自动判断优先级）
				corelog.Infof("HTTP long polling: [WRITE_FLUSH_LOOP] pushing complete packet to priority queue, size=%d, clientID=%d, mappingID=%s",
					packetSize, c.GetClientID(), c.GetMappingID())
				c.pollDataQueue.Push(data)

				// 立即通知 pollDataScheduler 有数据可用（非阻塞）
				select {
				case c.pollWaitChan <- struct{}{}:
				default:
				}
			} else {
				// 数据不完整，等待更多数据
				c.writeBufMu.Unlock()
			}
		} else {
			c.writeBufMu.Unlock()
		}
	}
}

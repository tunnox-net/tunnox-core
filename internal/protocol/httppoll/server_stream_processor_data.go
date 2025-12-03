package httppoll

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"tunnox-core/internal/utils"
)

// WriteExact 将数据流写入 Poll 响应（支持分片）
func (sp *ServerStreamProcessor) WriteExact(data []byte) error {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return io.ErrClosedPipe
	}

	// 分片数据
	fragments, err := SplitDataIntoFragments(data)
	if err != nil {
		utils.Errorf("ServerStreamProcessor[%s]: WriteExact failed to split data into fragments: %v", sp.connectionID, err)
		return err
	}

	// 序列化每个分片并推送到队列
	for _, fragment := range fragments {
		fragmentJSON, err := MarshalFragmentResponse(fragment)
		if err != nil {
			utils.Errorf("ServerStreamProcessor[%s]: WriteExact failed to marshal fragment: %v", sp.connectionID, err)
			return err
		}

		sp.pollDataQueue.Push(fragmentJSON)
	}

	// 通知等待的 Poll 请求
	select {
	case sp.pollWaitChan <- struct{}{}:
		utils.Debugf("ServerStreamProcessor[%s]: WriteExact notified pollWaitChan", sp.connectionID)
	default:
		utils.Debugf("ServerStreamProcessor[%s]: WriteExact pollWaitChan full", sp.connectionID)
	}

	return nil
}

// ReadAvailable 读取可用数据（不等待完整长度，用于适配 io.Reader）
// 支持分片重组：接收分片数据，重组后返回完整数据
func (sp *ServerStreamProcessor) ReadAvailable(maxLength int) ([]byte, error) {
	// 原子操作：检查并读取缓冲区（避免检查和读取之间的竞态条件）
	sp.readBufMu.Lock()
	bufferLen := len(sp.readBuffer)
	if bufferLen > 0 {
		// 如果缓冲区数据量较小（小于 maxLength），直接返回全部数据
		// 这样可以避免在 SSL/TLS 记录中间切分，导致解密失败
		// 只有当缓冲区数据量很大时，才进行切分
		readLen := bufferLen
		if readLen > maxLength {
			// 如果缓冲区数据量很大，优先返回 maxLength，但保留剩余数据
			readLen = maxLength
		}
		data := make([]byte, readLen)
		n := copy(data, sp.readBuffer[:readLen])
		sp.readBuffer = sp.readBuffer[n:]
		sp.readBufMu.Unlock()
		return data, nil
	}
	sp.readBufMu.Unlock()

	// 缓冲区为空，阻塞等待数据（使用合理的超时，平衡响应速度和数据完整性）
	// 对于大数据包，需要更长的超时时间以确保所有分片都能到达
	timeout := time.NewTimer(5 * time.Second) // 5秒超时，给大数据包分片足够的时间到达
	defer timeout.Stop()

	select {
	case <-sp.Ctx().Done():
		return nil, sp.Ctx().Err()
	case base64Data, ok := <-sp.pushDataChan:
		timeout.Stop()
		if !ok {
			return nil, io.EOF
		}

		// 解码Base64数据
		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			utils.Errorf("ServerStreamProcessor[%s]: ReadAvailable failed to decode base64: %v", sp.connectionID, err)
			return nil, fmt.Errorf("failed to decode base64: %w", err)
		}

		sp.readBufMu.Lock()
		sp.readBuffer = append(sp.readBuffer, data...)

		// 返回可用数据（最多 maxLength 字节）
		readLen := len(sp.readBuffer)
		if readLen > maxLength {
			readLen = maxLength
		}
		dataToReturn := make([]byte, readLen)
		n := copy(dataToReturn, sp.readBuffer[:readLen])
		sp.readBuffer = sp.readBuffer[n:]
		sp.readBufMu.Unlock()
		return dataToReturn, nil
	case <-timeout.C:
		// 超时，返回空数据（但不返回错误，让 io.Copy 重试）
		return nil, nil // 返回空，让 io.Copy 重试
	}
}

// ReadExact 从 Push 请求读取数据流
func (sp *ServerStreamProcessor) ReadExact(length int) ([]byte, error) {
	sp.readBufMu.Lock()
	defer sp.readBufMu.Unlock()

	// 从缓冲读取，如果不够则等待更多数据
	// ReadExact 应该等待完整数据，而不是返回部分数据（对于MySQL等协议，数据包必须完整）
	for len(sp.readBuffer) < length {
		sp.readBufMu.Unlock()

		// 使用合理的超时时间，平衡响应速度和数据完整性
		// 对于MySQL等协议，数据包通常较小，但需要等待完整数据包
		timeout := time.NewTimer(30 * time.Second) // 30秒超时，等待完整数据包
		select {
		case <-sp.Ctx().Done():
			timeout.Stop()
			return nil, sp.Ctx().Err()
		case base64Data, ok := <-sp.pushDataChan:
			timeout.Stop()
			if !ok {
				sp.readBufMu.Lock()
				// 如果缓冲区有数据但不够，返回部分数据（连接已关闭）
				if len(sp.readBuffer) > 0 {
					readLen := len(sp.readBuffer)
					data := make([]byte, readLen)
					n := copy(data, sp.readBuffer)
					sp.readBuffer = sp.readBuffer[n:]
					return data, io.EOF
				}
				return nil, io.EOF
			}
			// Base64 解码
			data, err := base64.StdEncoding.DecodeString(base64Data)
			if err != nil {
				sp.readBufMu.Lock()
				return nil, fmt.Errorf("failed to decode base64: %w", err)
			}
			sp.readBufMu.Lock()
			sp.readBuffer = append(sp.readBuffer, data...)
		case <-timeout.C:
			// 超时，检查缓冲区状态
			sp.readBufMu.Lock()
			if len(sp.readBuffer) >= length {
				// 缓冲区已经有足够的数据，继续循环以读取完整数据
				continue
			}
			// 如果缓冲区有数据但不够，继续等待（不返回部分数据）
			// 对于MySQL等协议，数据包必须完整，返回部分数据会导致序列号错误
			if len(sp.readBuffer) > 0 {
				continue
			}
			// 如果缓冲区为空，继续等待
			continue
		}
	}

	// 读取指定长度（现在缓冲区应该有足够的数据）
	data := make([]byte, length)
	n := copy(data, sp.readBuffer)
	sp.readBuffer = sp.readBuffer[n:]

	if n < length {
		// 不应该发生，因为我们已经等待了足够的数据
		return nil, io.ErrUnexpectedEOF
	}

	return data, nil
}

// PushData 推送数据到读取缓冲区（从 HTTP Push 请求调用）
func (sp *ServerStreamProcessor) PushData(base64Data string) error {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return io.ErrClosedPipe
	}

	select {
	case sp.pushDataChan <- base64Data:
		return nil
	case <-sp.Ctx().Done():
		return sp.Ctx().Err()
	default:
		return fmt.Errorf("push data channel full")
	}
}

// PollData 从数据队列获取数据（用于 Poll 响应）
func (sp *ServerStreamProcessor) PollData(ctx context.Context) (string, error) {
	// 先检查队列中是否有数据（非阻塞）
	if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
		// 返回分片响应的JSON字符串
		return string(fragmentJSON), nil
	}

	// 队列为空，等待数据（带超时）
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-sp.Ctx().Done():
		return "", sp.Ctx().Err()
	case <-sp.pollWaitChan:
		// 收到信号，立即检查队列
		if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
			return string(fragmentJSON), nil
		}
		// 如果队列仍为空，继续等待 pollDataChan
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-sp.Ctx().Done():
			return "", sp.Ctx().Err()
		case data, ok := <-sp.pollDataChan:
			if !ok {
				return "", io.EOF
			}
			// pollDataChan 中的数据已经是 JSON 字节数组
			return string(data), nil
		}
	case data, ok := <-sp.pollDataChan:
		if !ok {
			return "", io.EOF
		}
		// pollDataChan 中的数据已经是 JSON 字节数组
		return string(data), nil
	}
}

// pollDataScheduler 优先级队列调度循环
func (sp *ServerStreamProcessor) pollDataScheduler() {
	ticker := time.NewTicker(5 * time.Millisecond) // 减少检查间隔，提高响应速度
	defer ticker.Stop()

	for {
		select {
		case <-sp.Ctx().Done():
			return
		case <-ticker.C:
			// 定期检查队列，如果有数据且 pollDataChan 有空闲，则推送
			// 持续推送直到队列为空或 channel 满
			for {
				data, ok := sp.pollDataQueue.Pop()
				if !ok {
					break // 队列为空
				}
				select {
				case <-sp.Ctx().Done():
					// 如果 context 取消，将数据放回队列
					sp.pollDataQueue.Push(data)
					return
				case sp.pollDataChan <- data:
					// 通知 PollData 有数据可用（非阻塞，避免丢失通知）
					select {
					case sp.pollWaitChan <- struct{}{}:
					default:
						// pollWaitChan 已满，忽略（已有通知在等待）
					}
					// 继续推送下一个数据包
				default:
					// pollDataChan 已满（有数据正在等待），将数据放回队列（保持优先级）
					sp.pollDataQueue.Push(data)
					goto nextTick // 退出内层循环，等待下次 tick
				}
			}
		nextTick:
		}
	}
}


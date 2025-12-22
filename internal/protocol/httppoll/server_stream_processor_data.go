package httppoll

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"time"
)

// WriteExact 将数据流写入 Poll 响应（支持分片）
func (sp *ServerStreamProcessor) WriteExact(data []byte) error {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return io.ErrClosedPipe
	}

	sp.sequenceMu.Lock()
	sequenceNumber := sp.sequenceNumber
	sp.sequenceNumber++
	sp.sequenceMu.Unlock()

	fragments, err := SplitDataIntoFragments(data, sequenceNumber)
	if err != nil {
		return err
	}

	sp.writeMu.Lock()
	defer sp.writeMu.Unlock()

	for _, fragment := range fragments {
		fragmentJSON, err := MarshalFragmentResponse(fragment)
		if err != nil {
			return err
		}
		sp.pollDataQueue.Push(fragmentJSON)
	}

	select {
	case sp.pollWaitChan <- struct{}{}:
	default:
	}

	return nil
}

// ReadAvailable 读取可用数据（不等待完整长度）
func (sp *ServerStreamProcessor) ReadAvailable(maxLength int) ([]byte, error) {
	sp.readBufMu.Lock()
	bufferLen := len(sp.readBuffer)
	if bufferLen > 0 {
		readLen := bufferLen
		if readLen > maxLength {
			readLen = maxLength
		}
		data := make([]byte, readLen)
		n := copy(data, sp.readBuffer[:readLen])
		sp.readBuffer = sp.readBuffer[n:]
		sp.readBufMu.Unlock()
		return data, nil
	}
	sp.readBufMu.Unlock()

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	select {
	case <-sp.Ctx().Done():
		return nil, sp.Ctx().Err()
	case base64Data, ok := <-sp.pushDataChan:
		timeout.Stop()
		if !ok {
			return nil, io.EOF
		}

		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64: %w", err)
		}

		sp.readBufMu.Lock()
		sp.readBuffer = append(sp.readBuffer, data...)

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
		return nil, nil
	}
}

// ReadExact 从 Push 请求读取数据流
func (sp *ServerStreamProcessor) ReadExact(length int) ([]byte, error) {
	sp.readBufMu.Lock()
	defer sp.readBufMu.Unlock()

	for len(sp.readBuffer) < length {
		sp.readBufMu.Unlock()

		timeout := time.NewTimer(30 * time.Second)
		select {
		case <-sp.Ctx().Done():
			timeout.Stop()
			return nil, sp.Ctx().Err()
		case base64Data, ok := <-sp.pushDataChan:
			timeout.Stop()
			if !ok {
				sp.readBufMu.Lock()
				if len(sp.readBuffer) > 0 {
					readLen := len(sp.readBuffer)
					data := make([]byte, readLen)
					n := copy(data, sp.readBuffer)
					sp.readBuffer = sp.readBuffer[n:]
					return data, io.EOF
				}
				return nil, io.EOF
			}
			data, err := base64.StdEncoding.DecodeString(base64Data)
			if err != nil {
				sp.readBufMu.Lock()
				return nil, fmt.Errorf("failed to decode base64: %w", err)
			}
			sp.readBufMu.Lock()
			sp.readBuffer = append(sp.readBuffer, data...)
		case <-timeout.C:
			sp.readBufMu.Lock()
			if len(sp.readBuffer) >= length {
				continue
			}
			if len(sp.readBuffer) > 0 {
				continue
			}
			continue
		}
	}

	data := make([]byte, length)
	n := copy(data, sp.readBuffer)
	sp.readBuffer = sp.readBuffer[n:]

	if n < length {
		return nil, io.ErrUnexpectedEOF
	}

	return data, nil
}

// PushData 推送数据到读取缓冲区
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

// PollData 从数据队列获取数据
func (sp *ServerStreamProcessor) PollData(ctx context.Context) (string, error) {
	if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
		return string(fragmentJSON), nil
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-sp.Ctx().Done():
		return "", sp.Ctx().Err()
	case <-sp.pollWaitChan:
		if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
			return string(fragmentJSON), nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-sp.Ctx().Done():
			return "", sp.Ctx().Err()
		case data, ok := <-sp.pollDataChan:
			if !ok {
				return "", io.EOF
			}
			return string(data), nil
		}
	case data, ok := <-sp.pollDataChan:
		if !ok {
			return "", io.EOF
		}
		return string(data), nil
	}
}

// pollDataScheduler 优先级队列调度循环
func (sp *ServerStreamProcessor) pollDataScheduler() {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sp.Ctx().Done():
			return
		case <-ticker.C:
			for {
				data, ok := sp.pollDataQueue.Pop()
				if !ok {
					break
				}
				select {
				case <-sp.Ctx().Done():
					sp.pollDataQueue.Push(data)
					return
				case sp.pollDataChan <- data:
					select {
					case sp.pollWaitChan <- struct{}{}:
					default:
					}
				default:
					sp.pollDataQueue.Push(data)
					goto nextTick
				}
			}
		nextTick:
		}
	}
}

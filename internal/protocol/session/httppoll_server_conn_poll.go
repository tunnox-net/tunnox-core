package session

import (
	"context"
	"encoding/base64"
	"io"
	"time"
	corelog "tunnox-core/internal/core/log"
)

// PushData 从 HTTP POST 请求接收 Base64 编码的数据（由 handleHTTPPush 调用）
// 按照 Base64 适配层设计：Base64 数据直接发送到 base64PushDataChan
// Read() 方法会从 base64PushDataChan 接收并解码，追加到 readBuffer
func (c *ServerHTTPLongPollingConn) PushData(base64Data string) error {
	c.closeMu.RLock()
	closed := c.closed
	c.closeMu.RUnlock()

	if closed {
		corelog.Warnf("HTTP long polling: [PUSHDATA] connection closed, clientID=%d", c.GetClientID())
		return io.ErrClosedPipe
	}

	corelog.Infof("HTTP long polling: [PUSHDATA] pushing Base64 data (len=%d) to base64PushDataChan, clientID=%d",
		len(base64Data), c.GetClientID())
	select {
	case <-c.Ctx().Done():
		corelog.Warnf("HTTP long polling: [PUSHDATA] context canceled, clientID=%d", c.GetClientID())
		return c.Ctx().Err()
	case c.base64PushDataChan <- base64Data:
		corelog.Debugf("HTTP long polling: [PUSHDATA] Base64 data pushed successfully, clientID=%d", c.GetClientID())
		return nil
	default:
		corelog.Errorf("HTTP long polling: [PUSHDATA] base64PushDataChan full, clientID=%d", c.GetClientID())
		return io.ErrShortWrite
	}
}

// pollDataScheduler 优先级队列调度循环（将队列中的数据推送到 pollDataChan）
func (c *ServerHTTPLongPollingConn) pollDataScheduler() {
	corelog.Infof("HTTP long polling: [POLLDATA_SCHEDULER] started, clientID=%d", c.GetClientID())
	ticker := time.NewTicker(10 * time.Millisecond) // 每10ms检查一次队列
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx().Done():
			corelog.Debugf("HTTP long polling: [POLLDATA_SCHEDULER] context canceled, clientID=%d", c.GetClientID())
			return
		case <-ticker.C:
			// 定期检查队列，如果有数据且 pollDataChan 为空，则推送
			// 持续推送直到队列为空或 channel 满
			c.processQueueData()
		}
	}
}

// processQueueData 处理队列中的数据（从 pollDataScheduler 调用）
func (c *ServerHTTPLongPollingConn) processQueueData() {
	for {
		data, ok := c.pollDataQueue.Pop()
		if !ok {
			return // 队列为空
		}
		select {
		case <-c.Ctx().Done():
			// 如果 context 取消，将数据放回队列
			c.pollDataQueue.Push(data)
			return
		case c.pollDataChan <- data:
			corelog.Infof("HTTP long polling: [POLLDATA_SCHEDULER] pushed %d bytes to pollDataChan, queueLen=%d, clientID=%d, mappingID=%s",
				len(data), c.pollDataQueue.Len(), c.GetClientID(), c.GetMappingID())
			// 通知 PollData 有数据可用（非阻塞）
			select {
			case c.pollWaitChan <- struct{}{}:
			default:
			}
			// 继续推送下一个数据包
		default:
			// pollDataChan 已满（有数据正在等待），将数据放回队列（保持优先级）
			c.pollDataQueue.Push(data)
			return // 退出，等待下次 tick
		}
	}
}

// PollData 等待数据用于 HTTP GET 响应（由 handleHTTPPoll 调用）
// 返回 Base64 编码的数据，按照 Base64 适配层设计
func (c *ServerHTTPLongPollingConn) PollData(ctx context.Context) (string, error) {
	queueLen := c.pollDataQueue.Len()
	clientID := c.GetClientID()
	corelog.Infof("HTTP long polling: [POLLDATA] waiting for data, clientID=%d, queueLen=%d",
		clientID, queueLen)

	// 先检查队列中是否有数据（非阻塞）
	if data, ok := c.pollDataQueue.Pop(); ok {
		corelog.Infof("HTTP long polling: [POLLDATA] received %d bytes from queue, encoding to Base64, clientID=%d",
			len(data), clientID)
		base64Data := base64.StdEncoding.EncodeToString(data)
		return base64Data, nil
	}

	// 队列为空，阻塞等待调度器推送数据
	// 使用 select 同时监听 pollDataChan 和 pollWaitChan
	select {
	case <-ctx.Done():
		corelog.Debugf("HTTP long polling: [POLLDATA] context canceled, clientID=%d", clientID)
		return "", ctx.Err()
	case <-c.Ctx().Done():
		corelog.Debugf("HTTP long polling: [POLLDATA] connection context canceled, clientID=%d", clientID)
		return "", c.Ctx().Err()
	case <-c.pollWaitChan:
		// 收到信号，立即检查队列（可能有数据被调度器推送）
		if data, ok := c.pollDataQueue.Pop(); ok {
			corelog.Infof("HTTP long polling: [POLLDATA] received %d bytes from queue (after signal), encoding to Base64, clientID=%d",
				len(data), clientID)
			base64Data := base64.StdEncoding.EncodeToString(data)
			return base64Data, nil
		}
		// 如果队列仍为空，继续等待 pollDataChan
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-c.Ctx().Done():
			return "", c.Ctx().Err()
		case data, ok := <-c.pollDataChan:
			if !ok {
				return "", io.EOF
			}
			corelog.Infof("HTTP long polling: [POLLDATA] received %d bytes from channel, encoding to Base64, clientID=%d",
				len(data), clientID)
			base64Data := base64.StdEncoding.EncodeToString(data)
			return base64Data, nil
		}
	case data, ok := <-c.pollDataChan:
		if !ok {
			corelog.Debugf("HTTP long polling: [POLLDATA] channel closed, clientID=%d", clientID)
			return "", io.EOF
		}
		corelog.Infof("HTTP long polling: [POLLDATA] received %d bytes from channel, encoding to Base64, clientID=%d",
			len(data), clientID)

		// Base64 编码
		base64Data := base64.StdEncoding.EncodeToString(data)
		return base64Data, nil
	}
}

package httppoll

// PopFromPollQueue 从 pollDataQueue 中取出数据
// 实现 session.PollDataQueueMigrator 接口
// 用于连接迁移时转移队列中的数据
func (sp *ServerStreamProcessor) PopFromPollQueue() ([]byte, bool) {
	return sp.pollDataQueue.Pop()
}

// PushToPollQueue 推送数据到 pollDataQueue
// 实现 session.PollDataQueueMigrator 接口
// 用于连接迁移时接收转移的数据
func (sp *ServerStreamProcessor) PushToPollQueue(data []byte) {
	sp.pollDataQueue.Push(data)
}

// NotifyPollDataAvailable 通知有新数据可用
// 实现 session.PollDataQueueMigrator 接口
// 用于连接迁移后唤醒等待的 poll 请求
func (sp *ServerStreamProcessor) NotifyPollDataAvailable() {
	// 非阻塞发送通知
	select {
	case sp.pollWaitChan <- struct{}{}:
		// 通知发送成功
	default:
		// 通道已满，说明已经有待处理的通知
	}
}

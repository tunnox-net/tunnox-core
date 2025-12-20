package stream

import (
	"context"
	"fmt"
	"io"
	corelog "tunnox-core/internal/core/log"
)

// StreamService 流服务适配器，让流管理器能够作为服务运行
type StreamService struct {
	manager *StreamManager
	name    string
	ctx     context.Context
}

// NewStreamService 创建流服务
func NewStreamService(name string, manager *StreamManager) *StreamService {
	return &StreamService{
		manager: manager,
		name:    name,
	}
}

// Name 实现Service接口
func (ss *StreamService) Name() string {
	return ss.name
}

// Start 启动流服务
func (ss *StreamService) Start(ctx context.Context) error {
	ss.ctx = ctx
	// 精简日志：只在调试模式下输出服务启动信息
	corelog.Debugf("Starting stream service: %s", ss.name)

	// 流管理器在创建时就已经初始化，这里主要是验证状态
	if ss.manager == nil {
		return fmt.Errorf("stream manager is nil for service %s", ss.name)
	}

	// 精简日志：只在调试模式下输出服务启动完成信息
	corelog.Debugf("Stream service started: %s", ss.name)
	return nil
}

// Stop 停止流服务
func (ss *StreamService) Stop(ctx context.Context) error {
	corelog.Infof("Stopping stream service: %s", ss.name)

	// 关闭所有流
	if err := ss.manager.CloseAllStreams(); err != nil {
		return fmt.Errorf("failed to stop stream service %s: %v", ss.name, err)
	}

	corelog.Infof("Stream service stopped: %s", ss.name)
	return nil
}

// GetManager 获取流管理器
func (ss *StreamService) GetManager() *StreamManager {
	return ss.manager
}

// CreateStream 创建流
func (ss *StreamService) CreateStream(name string, reader io.Reader, writer io.Writer) (PackageStreamer, error) {
	return ss.manager.CreateStream(name, reader, writer)
}

// GetStream 获取流
func (ss *StreamService) GetStream(name string) (PackageStreamer, bool) {
	return ss.manager.GetStream(name)
}

// RemoveStream 移除流
func (ss *StreamService) RemoveStream(name string) error {
	return ss.manager.RemoveStream(name)
}

// GetStreamCount 获取流数量
func (ss *StreamService) GetStreamCount() int {
	return ss.manager.GetStreamCount()
}

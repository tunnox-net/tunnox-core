package stream

import (
	"context"
	"fmt"
	"io"
	"sync"
	"tunnox-core/internal/core/dispose"
)

// StreamManager 流管理器
type StreamManager struct {
	*dispose.ManagerBase
	factory StreamFactory
	streams map[string]PackageStreamer
	mu      sync.RWMutex
}

// Dispose 实现Disposable接口
func (m *StreamManager) Dispose() error {
	return m.CloseAllStreams()
}

// NewStreamManager 创建新的流管理器
func NewStreamManager(factory StreamFactory, parentCtx context.Context) *StreamManager {
	manager := &StreamManager{
		ManagerBase: dispose.NewManager("StreamManager", parentCtx),
		factory:     factory,
		streams:     make(map[string]PackageStreamer),
	}
	return manager
}

// CreateStream 创建新的流并注册到管理器
func (m *StreamManager) CreateStream(id string, reader io.Reader, writer io.Writer) (PackageStreamer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在
	if _, exists := m.streams[id]; exists {
		return nil, fmt.Errorf("stream with id %s already exists", id)
	}

	// 检查 reader 或 writer 是否已经是 PackageStreamer（如 HTTP Long Polling 的 ServerStreamProcessor）
	// 注意：需要递归检查，因为可能被包装在适配器中
	var stream PackageStreamer
	if streamer, ok := reader.(PackageStreamer); ok {
		stream = streamer
	} else if streamer, ok := writer.(PackageStreamer); ok {
		stream = streamer
	} else {
		// 创建新流
		stream = m.factory.NewStreamProcessor(reader, writer)
	}

	m.streams[id] = stream

	return stream, nil
}

// GetStream 获取指定ID的流
func (m *StreamManager) GetStream(id string) (PackageStreamer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stream, exists := m.streams[id]
	return stream, exists
}

// RemoveStream 移除指定ID的流
func (m *StreamManager) RemoveStream(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stream, exists := m.streams[id]
	if !exists {
		return fmt.Errorf("stream with id %s not found", id)
	}

	// 关闭流
	stream.Close()
	delete(m.streams, id)

	return nil
}

// ListStreams 列出所有流的ID
func (m *StreamManager) ListStreams() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.streams))
	for id := range m.streams {
		ids = append(ids, id)
	}
	return ids
}

// GetStreamCount 获取流数量
func (m *StreamManager) GetStreamCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.streams)
}

// CloseAllStreams 关闭所有流
func (m *StreamManager) CloseAllStreams() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, stream := range m.streams {
		stream.Close()
	}
	m.streams = make(map[string]PackageStreamer)
	return nil
}

// StreamConfig 流配置
type StreamConfig struct {
	// 流ID
	ID string
	// 是否启用压缩
	EnableCompression bool
	// 限速设置（字节/秒）
	RateLimit int64
	// 缓冲区大小
	BufferSize int
}

// CreateStreamWithConfig 使用配置创建流
func (m *StreamManager) CreateStreamWithConfig(config StreamConfig, reader io.Reader, writer io.Writer) (PackageStreamer, error) {
	// 创建基础流
	stream, err := m.CreateStream(config.ID, reader, writer)
	if err != nil {
		return nil, err
	}

	// 应用配置
	if config.EnableCompression {
		// 这里可以创建包装的压缩流
		// 暂时返回基础流，后续可以扩展
	}

	return stream, nil
}

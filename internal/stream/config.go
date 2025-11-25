package stream

import (
	"context"
	"fmt"
)

// StreamType 流类型枚举
type StreamType string

const (
	StreamTypeBasic       StreamType = "basic"
	StreamTypeCompressed  StreamType = "compressed"
	StreamTypeRateLimited StreamType = "rate_limited"
	StreamTypeHybrid      StreamType = "hybrid"
)

// StreamProfile 流配置模板
type StreamProfile struct {
	Name                 string
	Type                 StreamType
	DefaultCompression   bool
	DefaultRateLimit     int64
	BufferSize           int
	EnableMemoryPool     bool
	MaxConcurrentStreams int
	// 注意：加密功能已移至 internal/stream/transform 模块
	// 使用 transform.TransformConfig 配置加密
}

// PredefinedProfiles 预定义的流配置模板
var PredefinedProfiles = map[string]StreamProfile{
	"default": {
		Name:                 "default",
		Type:                 StreamTypeBasic,
		DefaultCompression:   false,
		DefaultRateLimit:     0,
		BufferSize:           4096,
		EnableMemoryPool:     true,
		MaxConcurrentStreams: 1000,
	},
	"high_performance": {
		Name:                 "high_performance",
		Type:                 StreamTypeHybrid,
		DefaultCompression:   true,
		DefaultRateLimit:     0,
		BufferSize:           8192,
		EnableMemoryPool:     true,
		MaxConcurrentStreams: 5000,
	},
	"bandwidth_saving": {
		Name:                 "bandwidth_saving",
		Type:                 StreamTypeCompressed,
		DefaultCompression:   true,
		DefaultRateLimit:     1024 * 1024, // 1MB/s
		BufferSize:           2048,
		EnableMemoryPool:     true,
		MaxConcurrentStreams: 500,
	},
	"low_latency": {
		Name:                 "low_latency",
		Type:                 StreamTypeBasic,
		DefaultCompression:   false,
		DefaultRateLimit:     0,
		BufferSize:           1024,
		EnableMemoryPool:     false,
		MaxConcurrentStreams: 2000,
	},
	"encrypted": {
		Name:                 "encrypted",
		Type:                 StreamTypeHybrid,
		DefaultCompression:   true,
		DefaultRateLimit:     0,
		BufferSize:           4096,
		EnableMemoryPool:     true,
		MaxConcurrentStreams: 1000,
		// 注意：加密功能已移至 internal/stream/transform 模块
	},
}

// GetProfile 获取预定义配置模板
func GetProfile(name string) (StreamProfile, error) {
	profile, exists := PredefinedProfiles[name]
	if !exists {
		return StreamProfile{}, fmt.Errorf("profile %s not found", name)
	}
	return profile, nil
}

// CreateFactoryFromProfile 根据配置模板创建流工厂
func CreateFactoryFromProfile(ctx context.Context, profileName string) (StreamFactory, error) {
	profile, err := GetProfile(profileName)
	if err != nil {
		return nil, err
	}

	config := &StreamFactoryConfig{
		EnableCompression: profile.DefaultCompression,
		RateLimitBytes:    profile.DefaultRateLimit,
		BufferSize:        profile.BufferSize,
		// 注意：加密功能已移至 internal/stream/transform 模块
	}

	return NewConfigurableStreamFactory(ctx, config), nil
}

// CreateManagerFromProfile 根据配置模板创建流管理器
func CreateManagerFromProfile(ctx context.Context, profileName string) (*StreamManager, error) {
	factory, err := CreateFactoryFromProfile(ctx, profileName)
	if err != nil {
		return nil, err
	}

	return NewStreamManager(factory, ctx), nil
}

// StreamMetrics 流指标
type StreamMetrics struct {
	TotalStreams       int
	ActiveStreams      int
	CompressedStreams  int
	RateLimitedStreams int
	TotalBytesRead     int64
	TotalBytesWritten  int64
}

// GetMetrics 获取流管理器指标
func (m *StreamManager) GetMetrics() StreamMetrics {
	streams := m.ListStreams()

	metrics := StreamMetrics{
		TotalStreams:  len(streams),
		ActiveStreams: len(streams),
	}

	// 这里可以添加更详细的指标统计
	// 比如压缩流数量、限速流数量等

	return metrics
}

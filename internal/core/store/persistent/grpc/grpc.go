// Package grpc 提供 gRPC 持久化存储实现
package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"tunnox-core/internal/core/store"
)

// =============================================================================
// GRPCStore gRPC 持久化存储实现
// =============================================================================

// GRPCStore gRPC 持久化存储
// 通过 gRPC 与 tunnox-storage 服务通信
type GRPCStore[K comparable, V any] struct {
	conn      *grpc.ClientConn
	client    StorageServiceClient
	keyPrefix string
	timeout   time.Duration
	metrics   *store.StoreMetrics
}

// StorageServiceClient gRPC 存储服务客户端接口
// 与 tunnox-storage 的 proto 定义对应
type StorageServiceClient interface {
	Get(ctx context.Context, req *GetRequest) (*GetResponse, error)
	Set(ctx context.Context, req *SetRequest) (*SetResponse, error)
	Delete(ctx context.Context, req *DeleteRequest) (*DeleteResponse, error)
	Exists(ctx context.Context, req *ExistsRequest) (*ExistsResponse, error)
	BatchGet(ctx context.Context, req *BatchGetRequest) (*BatchGetResponse, error)
	BatchSet(ctx context.Context, req *BatchSetRequest) (*BatchSetResponse, error)
	BatchDelete(ctx context.Context, req *BatchDeleteRequest) (*BatchDeleteResponse, error)
	List(ctx context.Context, req *ListRequest) (*ListResponse, error)
}

// 请求/响应类型定义（简化版，实际应从 proto 生成）
type (
	GetRequest         struct{ Key string }
	GetResponse        struct{ Value []byte }
	SetRequest         struct{ Key string; Value []byte }
	SetResponse        struct{}
	DeleteRequest      struct{ Key string }
	DeleteResponse     struct{}
	ExistsRequest      struct{ Key string }
	ExistsResponse     struct{ Exists bool }
	BatchGetRequest    struct{ Keys []string }
	BatchGetResponse   struct{ Values map[string][]byte }
	BatchSetRequest    struct{ Items map[string][]byte }
	BatchSetResponse   struct{}
	BatchDeleteRequest struct{ Keys []string }
	BatchDeleteResponse struct{}
	ListRequest        struct{ Prefix string }
	ListResponse       struct{ Items map[string][]byte }
)

// NewGRPCStore 创建 gRPC 存储
func NewGRPCStore[K comparable, V any](
	address string,
	keyPrefix string,
	timeout time.Duration,
) (*GRPCStore[K, V], error) {
	// 建立 gRPC 连接
	conn, err := grpc.Dial(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial failed: %w", err)
	}

	return &GRPCStore[K, V]{
		conn:      conn,
		keyPrefix: keyPrefix,
		timeout:   timeout,
		metrics:   store.NewStoreMetrics(),
	}, nil
}

// NewGRPCStoreFromConfig 从配置创建 gRPC 存储
func NewGRPCStoreFromConfig[K comparable, V any](cfg *store.GRPCConfig, keyPrefix string) (*GRPCStore[K, V], error) {
	return NewGRPCStore[K, V](cfg.Address, keyPrefix, cfg.Timeout)
}

// buildKey 构建存储键
func (s *GRPCStore[K, V]) buildKey(key K) string {
	return fmt.Sprintf("%s%v", s.keyPrefix, key)
}

// serialize 序列化值
func (s *GRPCStore[K, V]) serialize(value V) ([]byte, error) {
	return json.Marshal(value)
}

// deserialize 反序列化值
func (s *GRPCStore[K, V]) deserialize(data []byte) (V, error) {
	var value V
	err := json.Unmarshal(data, &value)
	return value, err
}

// withTimeout 创建带超时的 context
func (s *GRPCStore[K, V]) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.timeout)
}

// Get 获取值
func (s *GRPCStore[K, V]) Get(ctx context.Context, key K) (V, error) {
	start := time.Now()
	var zero V

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	resp, err := s.client.Get(ctx, &GetRequest{Key: s.buildKey(key)})
	if err != nil {
		s.metrics.RecordGet(time.Since(start), err)
		return zero, store.NewStoreError("grpc", "Get", s.buildKey(key), err)
	}

	if resp.Value == nil {
		s.metrics.RecordGet(time.Since(start), store.ErrNotFound)
		return zero, store.ErrNotFound
	}

	value, err := s.deserialize(resp.Value)
	if err != nil {
		s.metrics.RecordGet(time.Since(start), err)
		return zero, store.NewStoreError("grpc", "Get", s.buildKey(key), store.ErrDeserializationFailed)
	}

	s.metrics.RecordGet(time.Since(start), nil)
	return value, nil
}

// Set 设置值
func (s *GRPCStore[K, V]) Set(ctx context.Context, key K, value V) error {
	start := time.Now()

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	data, err := s.serialize(value)
	if err != nil {
		s.metrics.RecordSet(time.Since(start), err)
		return store.NewStoreError("grpc", "Set", s.buildKey(key), store.ErrSerializationFailed)
	}

	_, err = s.client.Set(ctx, &SetRequest{Key: s.buildKey(key), Value: data})
	s.metrics.RecordSet(time.Since(start), err)
	if err != nil {
		return store.NewStoreError("grpc", "Set", s.buildKey(key), err)
	}
	return nil
}

// Delete 删除值
func (s *GRPCStore[K, V]) Delete(ctx context.Context, key K) error {
	start := time.Now()

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	_, err := s.client.Delete(ctx, &DeleteRequest{Key: s.buildKey(key)})
	s.metrics.RecordDelete(time.Since(start), err)
	if err != nil {
		return store.NewStoreError("grpc", "Delete", s.buildKey(key), err)
	}
	return nil
}

// Exists 检查键是否存在
func (s *GRPCStore[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	resp, err := s.client.Exists(ctx, &ExistsRequest{Key: s.buildKey(key)})
	if err != nil {
		return false, store.NewStoreError("grpc", "Exists", s.buildKey(key), err)
	}
	return resp.Exists, nil
}

// BatchGet 批量获取
func (s *GRPCStore[K, V]) BatchGet(ctx context.Context, keys []K) (map[K]V, error) {
	if len(keys) == 0 {
		return map[K]V{}, nil
	}

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 构建请求
	grpcKeys := make([]string, len(keys))
	keyMap := make(map[string]K, len(keys))
	for i, key := range keys {
		gkey := s.buildKey(key)
		grpcKeys[i] = gkey
		keyMap[gkey] = key
	}

	resp, err := s.client.BatchGet(ctx, &BatchGetRequest{Keys: grpcKeys})
	if err != nil {
		return nil, store.NewStoreError("grpc", "BatchGet", "", err)
	}

	result := make(map[K]V, len(resp.Values))
	for gkey, data := range resp.Values {
		value, err := s.deserialize(data)
		if err != nil {
			// 跳过反序列化失败的记录（可能是旧版数据格式）
			continue
		}
		if key, ok := keyMap[gkey]; ok {
			result[key] = value
		}
	}

	return result, nil
}

// BatchSet 批量设置
func (s *GRPCStore[K, V]) BatchSet(ctx context.Context, items map[K]V) error {
	if len(items) == 0 {
		return nil
	}

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	grpcItems := make(map[string][]byte, len(items))
	for key, value := range items {
		data, err := s.serialize(value)
		if err != nil {
			return store.NewStoreError("grpc", "BatchSet", s.buildKey(key), store.ErrSerializationFailed)
		}
		grpcItems[s.buildKey(key)] = data
	}

	_, err := s.client.BatchSet(ctx, &BatchSetRequest{Items: grpcItems})
	if err != nil {
		return store.NewStoreError("grpc", "BatchSet", "", err)
	}
	return nil
}

// BatchDelete 批量删除
func (s *GRPCStore[K, V]) BatchDelete(ctx context.Context, keys []K) error {
	if len(keys) == 0 {
		return nil
	}

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	grpcKeys := make([]string, len(keys))
	for i, key := range keys {
		grpcKeys[i] = s.buildKey(key)
	}

	_, err := s.client.BatchDelete(ctx, &BatchDeleteRequest{Keys: grpcKeys})
	if err != nil {
		return store.NewStoreError("grpc", "BatchDelete", "", err)
	}
	return nil
}

// List 列出所有键值对
func (s *GRPCStore[K, V]) List(ctx context.Context, prefix string) (map[K]V, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	fullPrefix := s.keyPrefix
	if prefix != "" {
		fullPrefix = s.keyPrefix + prefix
	}

	resp, err := s.client.List(ctx, &ListRequest{Prefix: fullPrefix})
	if err != nil {
		return nil, store.NewStoreError("grpc", "List", "", err)
	}

	result := make(map[K]V, len(resp.Items))
	for gkey, data := range resp.Items {
		value, err := s.deserialize(data)
		if err != nil {
			continue
		}
		// 注意：这里需要将 string 转换为 K 类型
		// 由于泛型限制，这里简化处理
		var key K
		// 尝试类型断言
		if k, ok := any(gkey).(K); ok {
			result[k] = value
		} else {
			_ = key // 忽略无法转换的键
		}
	}

	return result, nil
}

// Ping 健康检查
func (s *GRPCStore[K, V]) Ping(ctx context.Context) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 简单的存在性检查作为 ping
	_, err := s.client.Exists(ctx, &ExistsRequest{Key: "__ping__"})
	return err
}

// Close 关闭连接
func (s *GRPCStore[K, V]) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// GetMetrics 获取指标
func (s *GRPCStore[K, V]) GetMetrics() *store.StoreMetrics {
	return s.metrics
}

// SetClient 设置 gRPC 客户端（用于测试）
func (s *GRPCStore[K, V]) SetClient(client StorageServiceClient) {
	s.client = client
}

// =============================================================================
// PersistentStore 接口实现验证
// =============================================================================

// 确保 GRPCStore 实现了 PersistentStore 接口
var _ store.PersistentStore[string, string] = (*GRPCStore[string, string])(nil)

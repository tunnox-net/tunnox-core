package storage

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/core/dispose"
)

// RemoteStorageConfig 远程存储配置
type RemoteStorageConfig struct {
	// gRPC 服务地址
	GRPCAddress string
	
	// 连接超时
	Timeout time.Duration
	
	// 最大重试次数
	MaxRetries int
	
	// TLS 配置
	TLSEnabled bool
	TLSCertFile string
	TLSKeyFile string
}

// RemoteStorage 远程存储实现（通过 gRPC）
// 用于集群模式下的持久化存储
type RemoteStorage struct {
	config *RemoteStorageConfig
	ctx    context.Context
	dispose.Dispose
	
	// TODO: 添加 gRPC 客户端连接
	// client storagepb.StorageServiceClient
}

// NewRemoteStorage 创建远程存储
func NewRemoteStorage(parentCtx context.Context, config *RemoteStorageConfig) (*RemoteStorage, error) {
	if config == nil {
		return nil, fmt.Errorf("remote storage config is required")
	}
	
	// 设置默认值
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	
	storage := &RemoteStorage{
		config: config,
		ctx:    parentCtx,
	}
	
	storage.SetCtx(parentCtx, storage.onClose)
	
	// TODO: 建立 gRPC 连接
	// conn, err := grpc.DialContext(parentCtx, config.GRPCAddress, opts...)
	// storage.client = storagepb.NewStorageServiceClient(conn)
	
	dispose.Infof("RemoteStorage: initialized (gRPC: %s)", config.GRPCAddress)
	return storage, nil
}

// onClose 资源释放回调
func (r *RemoteStorage) onClose() error {
	dispose.Infof("RemoteStorage: closing")
	// TODO: 关闭 gRPC 连接
	return nil
}

// Set 设置键值对
func (r *RemoteStorage) Set(key string, value interface{}) error {
	// TODO: 实现 gRPC 调用
	// ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
	// defer cancel()
	// _, err := r.client.Set(ctx, &storagepb.SetRequest{Key: key, Value: ...})
	
	dispose.Debugf("RemoteStorage.Set: key=%s (not implemented)", key)
	return nil
}

// Get 获取值
func (r *RemoteStorage) Get(key string) (interface{}, error) {
	// TODO: 实现 gRPC 调用
	dispose.Debugf("RemoteStorage.Get: key=%s (not implemented)", key)
	return nil, ErrKeyNotFound
}

// Delete 删除键
func (r *RemoteStorage) Delete(key string) error {
	// TODO: 实现 gRPC 调用
	dispose.Debugf("RemoteStorage.Delete: key=%s (not implemented)", key)
	return nil
}

// Exists 检查键是否存在
func (r *RemoteStorage) Exists(key string) (bool, error) {
	// TODO: 实现 gRPC 调用
	dispose.Debugf("RemoteStorage.Exists: key=%s (not implemented)", key)
	return false, nil
}

// BatchSet 批量设置
func (r *RemoteStorage) BatchSet(items map[string]interface{}) error {
	// TODO: 实现 gRPC 调用
	dispose.Debugf("RemoteStorage.BatchSet: %d items (not implemented)", len(items))
	return nil
}

// BatchGet 批量获取
func (r *RemoteStorage) BatchGet(keys []string) (map[string]interface{}, error) {
	// TODO: 实现 gRPC 调用
	dispose.Debugf("RemoteStorage.BatchGet: %d keys (not implemented)", len(keys))
	return make(map[string]interface{}), nil
}

// BatchDelete 批量删除
func (r *RemoteStorage) BatchDelete(keys []string) error {
	// TODO: 实现 gRPC 调用
	dispose.Debugf("RemoteStorage.BatchDelete: %d keys (not implemented)", len(keys))
	return nil
}

// Close 关闭连接
func (r *RemoteStorage) Close() error {
	r.Dispose.Close()
	return nil
}


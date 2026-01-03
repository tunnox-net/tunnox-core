package remote

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	storagepb "tunnox-core/api/proto/storage"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage/types"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Config 远程存储配置
type Config struct {
	// gRPC 服务地址
	GRPCAddress string

	// 连接超时
	Timeout time.Duration

	// 最大重试次数
	MaxRetries int

	// TLS 配置
	TLSEnabled  bool
	TLSCertFile string
	TLSKeyFile  string
}

// Storage 远程存储实现（通过 gRPC）
// 用于集群模式下的持久化存储
type Storage struct {
	config *Config
	ctx    context.Context
	cancel context.CancelFunc
	dispose.Dispose

	// gRPC 客户端连接
	conn   *grpc.ClientConn
	client storagepb.StorageServiceClient

	// 连接状态
	connected bool
	connMu    sync.RWMutex
}

// New 创建远程存储
func New(parentCtx context.Context, config *Config) (*Storage, error) {
	if config == nil {
		return nil, fmt.Errorf("remote storage config is required")
	}

	if config.GRPCAddress == "" {
		return nil, fmt.Errorf("gRPC address is required")
	}

	// 设置默认值
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	ctx, cancel := context.WithCancel(parentCtx)

	storage := &Storage{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}

	storage.SetCtx(ctx, storage.onClose)

	// 建立 gRPC 连接
	if err := storage.connect(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to storage service: %w", err)
	}

	dispose.Infof("RemoteStorage: connected to %s", config.GRPCAddress)
	return storage, nil
}

// connect 建立 gRPC 连接
func (r *Storage) connect() error {
	r.connMu.Lock()
	defer r.connMu.Unlock()

	// 配置 gRPC 连接选项
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	// TLS 配置
	if r.config.TLSEnabled {
		// 如果提供了证书文件，使用客户端证书
		if r.config.TLSCertFile != "" {
			creds, err := credentials.NewClientTLSFromFile(r.config.TLSCertFile, "")
			if err != nil {
				return fmt.Errorf("failed to load TLS credentials from %s: %w", r.config.TLSCertFile, err)
			}
			opts = append(opts, grpc.WithTransportCredentials(creds))
			dispose.Debugf("RemoteStorage: using TLS with certificate file: %s", r.config.TLSCertFile)
		} else {
			// 使用系统证书池（不验证服务器证书）
			tlsConfig := &tls.Config{
				InsecureSkipVerify: false, // 生产环境应该验证证书
			}
			creds := credentials.NewTLS(tlsConfig)
			opts = append(opts, grpc.WithTransportCredentials(creds))
			dispose.Debugf("RemoteStorage: using TLS with system certificate pool")
		}
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// 建立连接
	ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, r.config.GRPCAddress, opts...)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	r.conn = conn
	r.client = storagepb.NewStorageServiceClient(conn)
	r.connected = true

	return nil
}

// ensureConnected 确保连接可用
func (r *Storage) ensureConnected() error {
	r.connMu.RLock()
	if r.connected && r.conn != nil {
		r.connMu.RUnlock()
		return nil
	}
	r.connMu.RUnlock()

	return r.connect()
}

// onClose 资源释放回调
func (r *Storage) onClose() error {
	dispose.Infof("RemoteStorage: closing")
	r.cancel()

	r.connMu.Lock()
	defer r.connMu.Unlock()

	if r.conn != nil {
		if err := r.conn.Close(); err != nil {
			dispose.Warnf("RemoteStorage: error closing connection: %v", err)
		}
		r.conn = nil
	}
	r.connected = false

	return nil
}

// withRetry 带重试的操作执行器
// 注意：ErrKeyNotFound 不会触发重试，因为它是正常的"未找到"响应
func (r *Storage) withRetry(operation func() error) error {
	var lastErr error
	for i := 0; i < r.config.MaxRetries; i++ {
		if err := r.ensureConnected(); err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			continue
		}

		if err := operation(); err != nil {
			// ErrKeyNotFound 是正常响应，不需要重试
			if err == types.ErrKeyNotFound {
				return err
			}
			lastErr = err
			// 检查是否是连接错误，如果是则重连
			r.connMu.Lock()
			r.connected = false
			r.connMu.Unlock()
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			continue
		}
		return nil
	}
	return fmt.Errorf("operation failed after %d retries: %w", r.config.MaxRetries, lastErr)
}

// Set 设置键值对
// 注意：如果 value 已经是 string 或 []byte，直接使用，避免双重 JSON 编码
func (r *Storage) Set(key string, value interface{}) error {
	var data []byte
	switch v := value.(type) {
	case string:
		// 值已经是字符串，直接使用（常见于已序列化的 JSON）
		data = []byte(v)
	case []byte:
		// 值已经是字节数组，直接使用
		data = v
	default:
		// 其他类型，序列化为 JSON
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}
	}

	return r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.Set(ctx, &storagepb.SetRequest{
			Key:   key,
			Value: data,
		})
		if err != nil {
			return err
		}
		if !resp.Success {
			return fmt.Errorf("set failed: %s", resp.Error)
		}
		return nil
	})
}

// Get 获取值
func (r *Storage) Get(key string) (interface{}, error) {
	var result interface{}

	err := r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.Get(ctx, &storagepb.GetRequest{Key: key})
		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("get failed: %s", resp.Error)
		}
		if !resp.Found {
			return types.ErrKeyNotFound
		}

		if err := json.Unmarshal(resp.Value, &result); err != nil {
			return fmt.Errorf("failed to unmarshal value: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// Delete 删除键
func (r *Storage) Delete(key string) error {
	return r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.Delete(ctx, &storagepb.DeleteRequest{Key: key})
		if err != nil {
			return err
		}
		if !resp.Success {
			return fmt.Errorf("delete failed: %s", resp.Error)
		}
		return nil
	})
}

// Exists 检查键是否存在
func (r *Storage) Exists(key string) (bool, error) {
	var exists bool

	err := r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.Exists(ctx, &storagepb.ExistsRequest{Key: key})
		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("exists check failed: %s", resp.Error)
		}
		exists = resp.Exists
		return nil
	})

	return exists, err
}

// BatchSet 批量设置
func (r *Storage) BatchSet(items map[string]interface{}) error {
	kvItems := make([]*storagepb.KeyValue, 0, len(items))
	for key, value := range items {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
		}
		kvItems = append(kvItems, &storagepb.KeyValue{
			Key:   key,
			Value: data,
		})
	}

	return r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.BatchSet(ctx, &storagepb.BatchSetRequest{Items: kvItems})
		if err != nil {
			return err
		}
		if !resp.Success {
			return fmt.Errorf("batch set failed: %s", resp.Error)
		}
		return nil
	})
}

// BatchGet 批量获取
func (r *Storage) BatchGet(keys []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	err := r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.BatchGet(ctx, &storagepb.BatchGetRequest{Keys: keys})
		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("batch get failed: %s", resp.Error)
		}

		for _, item := range resp.Items {
			var value interface{}
			if err := json.Unmarshal(item.Value, &value); err != nil {
				dispose.Warnf("RemoteStorage: failed to unmarshal value for key %s: %v", item.Key, err)
				continue
			}
			result[item.Key] = value
		}
		return nil
	})

	return result, err
}

// BatchDelete 批量删除
func (r *Storage) BatchDelete(keys []string) error {
	return r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.BatchDelete(ctx, &storagepb.BatchDeleteRequest{Keys: keys})
		if err != nil {
			return err
		}
		if !resp.Success {
			return fmt.Errorf("batch delete failed: %s", resp.Error)
		}
		return nil
	})
}

// QueryByField 按字段查询
func (r *Storage) QueryByField(keyPrefix string, fieldName string, fieldValue interface{}) ([]string, error) {
	valueData, err := json.Marshal(fieldValue)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal field value: %w", err)
	}

	var keys []string
	err = r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.QueryByField(ctx, &storagepb.QueryByFieldRequest{
			KeyPrefix:  keyPrefix,
			FieldName:  fieldName,
			FieldValue: valueData,
		})

		if err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("query by field failed: %s", resp.Error)
		}
		keys = resp.Keys
		return nil
	})

	if err != nil {
		return nil, err
	}
	return keys, nil
}

// QueryByPrefix 按前缀查询所有键值对
func (r *Storage) QueryByPrefix(prefix string, limit int) (map[string]string, error) {
	result := make(map[string]string)

	dispose.Debugf("RemoteStorage.QueryByPrefix: calling gRPC with prefix=%s, limit=%d", prefix, limit)

	err := r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.QueryByPrefix(ctx, &storagepb.QueryByPrefixRequest{
			Prefix: prefix,
			Limit:  int32(limit),
		})
		if err != nil {
			dispose.Errorf("RemoteStorage.QueryByPrefix: gRPC error=%v", err)
			return err
		}
		if resp.Error != "" {
			dispose.Errorf("RemoteStorage.QueryByPrefix: response error=%s", resp.Error)
			return fmt.Errorf("query by prefix failed: %s", resp.Error)
		}

		dispose.Debugf("RemoteStorage.QueryByPrefix: received %d items", len(resp.Items))
		for _, item := range resp.Items {
			result[item.Key] = string(item.Value)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	dispose.Infof("RemoteStorage.QueryByPrefix: returning %d items for prefix=%s", len(result), prefix)
	return result, nil
}

// Ping 健康检查
func (r *Storage) Ping() error {
	return r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.Ping(ctx, &storagepb.PingRequest{
			Timestamp: time.Now().UnixMilli(),
		})
		if err != nil {
			return err
		}
		if !resp.Ok {
			return fmt.Errorf("ping failed")
		}
		return nil
	})
}

// GetClientAddress 获取当前节点的外部地址（通过 storage 服务反射获取）
func (r *Storage) GetClientAddress() (string, error) {
	var clientAddr string

	err := r.withRetry(func() error {
		ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
		defer cancel()

		resp, err := r.client.Ping(ctx, &storagepb.PingRequest{
			Timestamp: time.Now().UnixMilli(),
		})
		if err != nil {
			return err
		}
		if !resp.Ok {
			return fmt.Errorf("ping failed")
		}
		clientAddr = resp.ClientAddress
		return nil
	})

	if err != nil {
		return "", err
	}
	return clientAddr, nil
}

// Close 关闭连接
func (r *Storage) Close() error {
	r.Dispose.Close()
	return nil
}

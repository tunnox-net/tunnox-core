package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockStorage 模拟存储实现
type mockStorage struct {
	existsErr error
}

func (m *mockStorage) Set(key string, value interface{}, ttl time.Duration) error {
	return nil
}

func (m *mockStorage) Get(key string) (interface{}, error) {
	return nil, nil
}

func (m *mockStorage) Delete(key string) error {
	return nil
}

func (m *mockStorage) Exists(key string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	return true, nil
}

func (m *mockStorage) SetExpiration(key string, ttl time.Duration) error {
	return nil
}

func (m *mockStorage) GetExpiration(key string) (time.Duration, error) {
	return 0, nil
}

func (m *mockStorage) CleanupExpired() error {
	return nil
}

func (m *mockStorage) Close() error {
	return nil
}

func TestStorageAdapter(t *testing.T) {
	t.Run("nil storage", func(t *testing.T) {
		adapter := NewStorageAdapter(nil)
		err := adapter.Ping(context.Background())
		if err == nil {
			t.Error("expected error for nil storage, got nil")
		}
		if err.Error() != "storage is nil" {
			t.Errorf("expected 'storage is nil', got %s", err.Error())
		}
	})

	t.Run("healthy storage", func(t *testing.T) {
		mockStorage := &mockStorage{existsErr: nil}
		adapter := NewStorageAdapter(mockStorage)
		err := adapter.Ping(context.Background())
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("unhealthy storage", func(t *testing.T) {
		mockStorage := &mockStorage{existsErr: errors.New("storage error")}
		adapter := NewStorageAdapter(mockStorage)
		err := adapter.Ping(context.Background())
		if err == nil {
			t.Error("expected error, got nil")
		}
		if err.Error() != "storage error" {
			t.Errorf("expected 'storage error', got %s", err.Error())
		}
	})
}

// 验证 StorageAdapter 实现了 StorageChecker 接口
var _ StorageChecker = NewStorageAdapter(&mockStorage{})

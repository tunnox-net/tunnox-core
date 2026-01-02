// Package store 提供统一的存储抽象层
package store

import (
	"errors"
	"fmt"
)

// 存储层错误定义
var (
	// ErrNotFound 键不存在
	ErrNotFound = errors.New("key not found")

	// ErrAlreadyExists 键已存在
	ErrAlreadyExists = errors.New("key already exists")

	// ErrInvalidType 类型不匹配
	ErrInvalidType = errors.New("invalid type")

	// ErrConnectionFailed 连接失败
	ErrConnectionFailed = errors.New("connection failed")

	// ErrTimeout 操作超时
	ErrTimeout = errors.New("operation timeout")

	// ErrClosed 存储已关闭
	ErrClosed = errors.New("store closed")

	// ErrInvalidKey 无效的键
	ErrInvalidKey = errors.New("invalid key")

	// ErrSerializationFailed 序列化失败
	ErrSerializationFailed = errors.New("serialization failed")

	// ErrDeserializationFailed 反序列化失败
	ErrDeserializationFailed = errors.New("deserialization failed")
)

// StoreError 存储层错误包装
type StoreError struct {
	Op       string // 操作名称
	Key      string // 相关键
	StoreType string // 存储类型
	Err      error  // 原始错误
}

func (e *StoreError) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("store %s: %s key=%s: %v", e.StoreType, e.Op, e.Key, e.Err)
	}
	return fmt.Sprintf("store %s: %s: %v", e.StoreType, e.Op, e.Err)
}

func (e *StoreError) Unwrap() error {
	return e.Err
}

// NewStoreError 创建存储错误
func NewStoreError(storeType, op, key string, err error) *StoreError {
	return &StoreError{
		Op:        op,
		Key:       key,
		StoreType: storeType,
		Err:       err,
	}
}

// IsNotFound 检查是否为 NotFound 错误
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExists 检查是否为 AlreadyExists 错误
func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

// IsConnectionFailed 检查是否为连接失败错误
func IsConnectionFailed(err error) bool {
	return errors.Is(err, ErrConnectionFailed)
}

// IsTimeout 检查是否为超时错误
func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout)
}

package errors

import (
	"errors"
	"testing"
)

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name:     "without cause",
			err:      New(CodeNotFound, "user not found"),
			expected: "[NOT_FOUND] user not found",
		},
		{
			name:     "with cause",
			err:      Wrap(errors.New("db error"), CodeStorageError, "failed to query"),
			expected: "[STORAGE_ERROR] failed to query: db error",
		},
		{
			name:     "formatted message",
			err:      Newf(CodeInvalidParam, "invalid port: %d", 99999),
			expected: "[INVALID_PARAM] invalid port: 99999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestError_Is(t *testing.T) {
	err1 := New(CodeNotFound, "user not found")
	err2 := New(CodeNotFound, "client not found")
	err3 := New(CodeAuthFailed, "auth failed")

	// 相同错误码应该匹配
	if !errors.Is(err1, err2) {
		t.Error("errors with same code should match")
	}

	// 不同错误码不应该匹配
	if errors.Is(err1, err3) {
		t.Error("errors with different code should not match")
	}

	// 使用哨兵错误
	if !errors.Is(err1, ErrNotFound) {
		t.Error("should match sentinel error with same code")
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("original error")
	wrapped := Wrap(cause, CodeInternal, "wrapped")

	if errors.Unwrap(wrapped) != cause {
		t.Error("Unwrap should return the cause")
	}
}

func TestError_WithDetail(t *testing.T) {
	err := New(CodeInvalidParam, "invalid port").
		WithDetailInt("port", 99999).
		WithDetailInt("max", 65535)

	portVal, ok := err.GetDetailInt("port")
	if !ok || portVal != 99999 {
		t.Error("detail 'port' should be 99999")
	}
	maxVal, ok := err.GetDetailInt("max")
	if !ok || maxVal != 65535 {
		t.Error("detail 'max' should be 65535")
	}

	// 测试字符串类型详情
	err2 := New(CodeInvalidParam, "invalid name").
		WithDetailString("field", "username").
		WithDetailString("reason", "too short")

	if err2.GetDetailString("field") != "username" {
		t.Error("detail 'field' should be 'username'")
	}
	if err2.GetDetailString("reason") != "too short" {
		t.Error("detail 'reason' should be 'too short'")
	}

	// 测试整数转字符串
	if err.GetDetailString("port") != "99999" {
		t.Error("GetDetailString for int should return '99999'")
	}
}

func TestGetCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{
			name:     "custom error",
			err:      New(CodeNotFound, "not found"),
			expected: CodeNotFound,
		},
		{
			name:     "wrapped error",
			err:      Wrap(errors.New("db"), CodeStorageError, "storage"),
			expected: CodeStorageError,
		},
		{
			name:     "standard error",
			err:      errors.New("standard"),
			expected: CodeInternal,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetCode(tt.err); got != tt.expected {
				t.Errorf("GetCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsCode(t *testing.T) {
	err := New(CodeNotFound, "not found")

	if !IsCode(err, CodeNotFound) {
		t.Error("IsCode should return true for matching code")
	}

	if IsCode(err, CodeAuthFailed) {
		t.Error("IsCode should return false for non-matching code")
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"generic not found", ErrNotFound, true},
		{"client not found", ErrClientNotFound, true},
		{"user not found", ErrUserNotFound, true},
		{"auth error", ErrAuthFailed, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.expected {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"timeout", ErrTimeout, true},
		{"unavailable", ErrUnavailable, true},
		{"network error", ErrNetworkError, true},
		{"rate limited", ErrRateLimited, true},
		{"not found", ErrNotFound, false},
		{"auth failed", ErrAuthFailed, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.expected {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

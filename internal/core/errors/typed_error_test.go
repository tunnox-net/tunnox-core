package errors

import (
	"errors"
	"testing"
)

func TestTypedError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *TypedError
		expected string
	}{
		{
			name: "error with message only",
			err: &TypedError{
				Type:    ErrorTypeNetwork,
				Message: "connection failed",
			},
			expected: "[network] connection failed",
		},
		{
			name: "error with wrapped error",
			err: &TypedError{
				Type:    ErrorTypeStorage,
				Message: "storage operation failed",
				Err:     errors.New("disk full"),
			},
			expected: "[storage] storage operation failed: disk full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("TypedError.Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTypedError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	typedErr := &TypedError{
		Type:    ErrorTypeNetwork,
		Message: "wrapped error",
		Err:     originalErr,
	}

	if unwrapped := typedErr.Unwrap(); unwrapped != originalErr {
		t.Errorf("TypedError.Unwrap() = %v, want %v", unwrapped, originalErr)
	}
}

func TestWrap(t *testing.T) {
	originalErr := errors.New("original error")
	wrapped := Wrap(originalErr, ErrorTypeNetwork, "network operation failed")

	typedErr, ok := wrapped.(*TypedError)
	if !ok {
		t.Fatalf("Wrap() did not return *TypedError, got %T", wrapped)
	}

	if typedErr.Type != ErrorTypeNetwork {
		t.Errorf("Wrap() Type = %v, want %v", typedErr.Type, ErrorTypeNetwork)
	}
	if typedErr.Message != "network operation failed" {
		t.Errorf("Wrap() Message = %v, want %v", typedErr.Message, "network operation failed")
	}
	if typedErr.Err != originalErr {
		t.Errorf("Wrap() Err = %v, want %v", typedErr.Err, originalErr)
	}
	if !typedErr.Retryable {
		t.Errorf("Wrap() Retryable = %v, want true", typedErr.Retryable)
	}
}

func TestWrap_Nil(t *testing.T) {
	wrapped := Wrap(nil, ErrorTypeNetwork, "test")
	if wrapped != nil {
		t.Errorf("Wrap(nil) = %v, want nil", wrapped)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "temporary error",
			err:      Wrap(errors.New("test"), ErrorTypeTemporary, "test"),
			expected: true,
		},
		{
			name:     "network error",
			err:      Wrap(errors.New("test"), ErrorTypeNetwork, "test"),
			expected: true,
		},
		{
			name:     "storage error",
			err:      Wrap(errors.New("test"), ErrorTypeStorage, "test"),
			expected: true,
		},
		{
			name:     "permanent error",
			err:      Wrap(errors.New("test"), ErrorTypePermanent, "test"),
			expected: false,
		},
		{
			name:     "auth error",
			err:      Wrap(errors.New("test"), ErrorTypeAuth, "test"),
			expected: false,
		},
		{
			name:     "fatal error",
			err:      Wrap(errors.New("test"), ErrorTypeFatal, "test"),
			expected: false,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.expected {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsAlertable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "protocol error",
			err:      Wrap(errors.New("test"), ErrorTypeProtocol, "test"),
			expected: true,
		},
		{
			name:     "storage error",
			err:      Wrap(errors.New("test"), ErrorTypeStorage, "test"),
			expected: true,
		},
		{
			name:     "auth error",
			err:      Wrap(errors.New("test"), ErrorTypeAuth, "test"),
			expected: true,
		},
		{
			name:     "fatal error",
			err:      Wrap(errors.New("test"), ErrorTypeFatal, "test"),
			expected: true,
		},
		{
			name:     "temporary error",
			err:      Wrap(errors.New("test"), ErrorTypeTemporary, "test"),
			expected: false,
		},
		{
			name:     "network error",
			err:      Wrap(errors.New("test"), ErrorTypeNetwork, "test"),
			expected: false,
		},
		{
			name:     "permanent error",
			err:      Wrap(errors.New("test"), ErrorTypePermanent, "test"),
			expected: false,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAlertable(tt.err); got != tt.expected {
				t.Errorf("IsAlertable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorType
	}{
		{
			name:     "typed error",
			err:      Wrap(errors.New("test"), ErrorTypeNetwork, "test"),
			expected: ErrorTypeNetwork,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: ErrorTypePermanent,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: ErrorTypePermanent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetErrorType(tt.err); got != tt.expected {
				t.Errorf("GetErrorType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	err := New(ErrorTypeNetwork, "connection failed")
	if err.Type != ErrorTypeNetwork {
		t.Errorf("New() Type = %v, want %v", err.Type, ErrorTypeNetwork)
	}
	if err.Message != "connection failed" {
		t.Errorf("New() Message = %v, want %v", err.Message, "connection failed")
	}
	if !err.Retryable {
		t.Errorf("New() Retryable = %v, want true", err.Retryable)
	}
}

func TestNewf(t *testing.T) {
	err := Newf(ErrorTypeStorage, "storage %s failed", "operation")
	if err.Type != ErrorTypeStorage {
		t.Errorf("Newf() Type = %v, want %v", err.Type, ErrorTypeStorage)
	}
	expectedMsg := "storage operation failed"
	if err.Message != expectedMsg {
		t.Errorf("Newf() Message = %v, want %v", err.Message, expectedMsg)
	}
	if !err.Retryable {
		t.Errorf("Newf() Retryable = %v, want true", err.Retryable)
	}
	if !err.Alertable {
		t.Errorf("Newf() Alertable = %v, want true", err.Alertable)
	}
}


package base

import (
	"errors"
	"testing"
	"time"

	coreerrors "tunnox-core/internal/core/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	service := NewService()
	assert.NotNil(t, service)
}

func TestService_HandleErrorWithIDRelease(t *testing.T) {
	service := NewService()

	tests := []struct {
		name          string
		err           error
		id            interface{}
		releaseFunc   func(interface{}) error
		message       string
		expectNil     bool
		releaseCount  int
	}{
		{
			name:         "nil error returns nil",
			err:          nil,
			id:           "test-id",
			releaseFunc:  nil,
			message:      "test message",
			expectNil:    true,
			releaseCount: 0,
		},
		{
			name:         "error without release func",
			err:          errors.New("test error"),
			id:           "test-id",
			releaseFunc:  nil,
			message:      "test message",
			expectNil:    false,
			releaseCount: 0,
		},
		{
			name: "error with release func",
			err:  errors.New("test error"),
			id:   "test-id",
			releaseFunc: func(id interface{}) error {
				return nil
			},
			message:      "test message",
			expectNil:    false,
			releaseCount: 1,
		},
		{
			name: "error with failing release func",
			err:  errors.New("test error"),
			id:   "test-id",
			releaseFunc: func(id interface{}) error {
				return errors.New("release failed")
			},
			message:      "test message",
			expectNil:    false,
			releaseCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			releaseCount := 0
			var releaseFunc func(interface{}) error
			if tt.releaseFunc != nil {
				releaseFunc = func(id interface{}) error {
					releaseCount++
					return tt.releaseFunc(id)
				}
			}

			result := service.HandleErrorWithIDRelease(tt.err, tt.id, releaseFunc, tt.message)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Contains(t, result.Error(), tt.message)
			}
			if tt.releaseFunc != nil {
				assert.Equal(t, tt.releaseCount, releaseCount)
			}
		})
	}
}

func TestService_HandleErrorWithIDReleaseInt64(t *testing.T) {
	service := NewService()

	tests := []struct {
		name        string
		err         error
		id          int64
		releaseFunc func(int64) error
		message     string
		expectNil   bool
	}{
		{
			name:        "nil error returns nil",
			err:         nil,
			id:          123,
			releaseFunc: nil,
			message:     "test message",
			expectNil:   true,
		},
		{
			name:        "error without release func",
			err:         errors.New("test error"),
			id:          123,
			releaseFunc: nil,
			message:     "test message",
			expectNil:   false,
		},
		{
			name: "error with successful release",
			err:  errors.New("test error"),
			id:   456,
			releaseFunc: func(id int64) error {
				return nil
			},
			message:   "test message",
			expectNil: false,
		},
		{
			name: "error with failed release",
			err:  errors.New("test error"),
			id:   789,
			releaseFunc: func(id int64) error {
				return errors.New("release failed")
			},
			message:   "test message",
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.HandleErrorWithIDReleaseInt64(tt.err, tt.id, tt.releaseFunc, tt.message)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Contains(t, result.Error(), tt.message)
			}
		})
	}
}

func TestService_HandleErrorWithIDReleaseString(t *testing.T) {
	service := NewService()

	tests := []struct {
		name        string
		err         error
		id          string
		releaseFunc func(string) error
		message     string
		expectNil   bool
	}{
		{
			name:        "nil error returns nil",
			err:         nil,
			id:          "test-id",
			releaseFunc: nil,
			message:     "test message",
			expectNil:   true,
		},
		{
			name:        "error without release func",
			err:         errors.New("test error"),
			id:          "test-id",
			releaseFunc: nil,
			message:     "test message",
			expectNil:   false,
		},
		{
			name: "error with successful release",
			err:  errors.New("test error"),
			id:   "test-id",
			releaseFunc: func(id string) error {
				return nil
			},
			message:   "test message",
			expectNil: false,
		},
		{
			name: "error with failed release",
			err:  errors.New("test error"),
			id:   "failed-id",
			releaseFunc: func(id string) error {
				return errors.New("release failed")
			},
			message:   "test message",
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.HandleErrorWithIDReleaseString(tt.err, tt.id, tt.releaseFunc, tt.message)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Contains(t, result.Error(), tt.message)
			}
		})
	}
}

func TestService_WrapError(t *testing.T) {
	service := NewService()

	tests := []struct {
		name      string
		err       error
		operation string
		expectNil bool
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			operation: "test operation",
			expectNil: true,
		},
		{
			name:      "generic error is wrapped",
			err:       errors.New("original error"),
			operation: "create client",
			expectNil: false,
		},
		{
			name:      "typed error preserves code",
			err:       coreerrors.New(coreerrors.CodeNotFound, "not found"),
			operation: "get client",
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.WrapError(tt.err, tt.operation)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Contains(t, result.Error(), tt.operation)
			}
		})
	}
}

func TestService_WrapErrorWithID(t *testing.T) {
	service := NewService()

	tests := []struct {
		name      string
		err       error
		operation string
		id        string
		expectNil bool
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			operation: "test operation",
			id:        "test-id",
			expectNil: true,
		},
		{
			name:      "error is wrapped with id",
			err:       errors.New("original error"),
			operation: "get",
			id:        "client-123",
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.WrapErrorWithID(tt.err, tt.operation, tt.id)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Contains(t, result.Error(), tt.operation)
				assert.Contains(t, result.Error(), tt.id)
			}
		})
	}
}

func TestService_WrapErrorWithInt64ID(t *testing.T) {
	service := NewService()

	tests := []struct {
		name      string
		err       error
		operation string
		id        int64
		expectNil bool
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			operation: "test operation",
			id:        123,
			expectNil: true,
		},
		{
			name:      "error is wrapped with int64 id",
			err:       errors.New("original error"),
			operation: "delete",
			id:        456789,
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.WrapErrorWithInt64ID(tt.err, tt.operation, tt.id)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Contains(t, result.Error(), tt.operation)
				assert.Contains(t, result.Error(), "456789")
			}
		})
	}
}

func TestService_LogMethods(t *testing.T) {
	service := NewService()

	// These methods just log, so we test they don't panic
	t.Run("LogCreated does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			service.LogCreated("client", "client-123")
		})
	})

	t.Run("LogUpdated does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			service.LogUpdated("mapping", "map-456")
		})
	})

	t.Run("LogDeleted does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			service.LogDeleted("user", "user-789")
		})
	})

	t.Run("LogWarning without args does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			service.LogWarning("create client", errors.New("test error"))
		})
	})

	t.Run("LogWarning with args does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			service.LogWarning("update %s", errors.New("test error"), "client")
		})
	})
}

func TestService_SetTimestamps(t *testing.T) {
	service := NewService()

	tests := []struct {
		name          string
		createdAt     *time.Time
		updatedAt     *time.Time
		expectCreated bool
		expectUpdated bool
	}{
		{
			name:          "set both timestamps",
			createdAt:     new(time.Time),
			updatedAt:     new(time.Time),
			expectCreated: true,
			expectUpdated: true,
		},
		{
			name:          "set only createdAt",
			createdAt:     new(time.Time),
			updatedAt:     nil,
			expectCreated: true,
			expectUpdated: false,
		},
		{
			name:          "set only updatedAt",
			createdAt:     nil,
			updatedAt:     new(time.Time),
			expectCreated: false,
			expectUpdated: true,
		},
		{
			name:          "nil both",
			createdAt:     nil,
			updatedAt:     nil,
			expectCreated: false,
			expectUpdated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			service.SetTimestamps(tt.createdAt, tt.updatedAt)
			after := time.Now()

			if tt.expectCreated {
				require.NotNil(t, tt.createdAt)
				assert.True(t, tt.createdAt.After(before) || tt.createdAt.Equal(before))
				assert.True(t, tt.createdAt.Before(after) || tt.createdAt.Equal(after))
			}
			if tt.expectUpdated {
				require.NotNil(t, tt.updatedAt)
				assert.True(t, tt.updatedAt.After(before) || tt.updatedAt.Equal(before))
				assert.True(t, tt.updatedAt.Before(after) || tt.updatedAt.Equal(after))
			}
		})
	}
}

func TestService_SetUpdatedTimestamp(t *testing.T) {
	service := NewService()

	t.Run("set updated timestamp", func(t *testing.T) {
		var ts time.Time
		before := time.Now()
		service.SetUpdatedTimestamp(&ts)
		after := time.Now()

		assert.True(t, ts.After(before) || ts.Equal(before))
		assert.True(t, ts.Before(after) || ts.Equal(after))
	})

	t.Run("nil pointer does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			service.SetUpdatedTimestamp(nil)
		})
	})
}

func TestSimpleStatsProvider(t *testing.T) {
	// Test with nil storage - should return a degraded provider
	t.Run("GetCounter returns nil in degraded mode", func(t *testing.T) {
		provider := &simpleStatsProvider{counter: nil}
		assert.Nil(t, provider.GetCounter())
	})

	t.Run("GetUserStats returns basic stats", func(t *testing.T) {
		provider := &simpleStatsProvider{counter: nil}
		stats, err := provider.GetUserStats("user-123")
		require.NoError(t, err)
		assert.Equal(t, "user-123", stats.UserID)
	})

	t.Run("GetClientStats returns basic stats", func(t *testing.T) {
		provider := &simpleStatsProvider{counter: nil}
		stats, err := provider.GetClientStats(123)
		require.NoError(t, err)
		assert.Equal(t, int64(123), stats.ClientID)
	})
}

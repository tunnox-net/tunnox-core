package dispose

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResourceBase(t *testing.T) {
	tests := []struct {
		name         string
		resourceName string
	}{
		{"create with simple name", "test-resource"},
		{"create with empty name", ""},
		{"create with complex name", "my-service-v1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := NewResourceBase(tt.resourceName)
			assert.NotNil(t, rb)
			assert.Equal(t, tt.resourceName, rb.name)
		})
	}
}

func TestResourceBase_Initialize(t *testing.T) {
	t.Run("initialize with background context", func(t *testing.T) {
		rb := NewResourceBase("test")
		ctx := context.Background()

		rb.Initialize(ctx)

		assert.NotNil(t, rb.ctx)
		assert.False(t, rb.IsClosed())
	})

	t.Run("initialize with cancellable context", func(t *testing.T) {
		rb := NewResourceBase("test")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rb.Initialize(ctx)

		assert.NotNil(t, rb.ctx)
		assert.NotNil(t, rb.cancel)
	})

	t.Run("initialize triggers cleanup on context cancel", func(t *testing.T) {
		rb := NewResourceBase("test")
		ctx, cancel := context.WithCancel(context.Background())

		rb.Initialize(ctx)

		// Give goroutine time to start
		time.Sleep(10 * time.Millisecond)

		cancel()

		// Wait for cleanup to trigger
		time.Sleep(50 * time.Millisecond)

		assert.True(t, rb.IsClosed())
	})
}

func TestResourceBase_Close(t *testing.T) {
	t.Run("close without initialization", func(t *testing.T) {
		rb := NewResourceBase("test")

		err := rb.Close()
		assert.NoError(t, err)
	})

	t.Run("close after initialization", func(t *testing.T) {
		rb := NewResourceBase("test")
		rb.Initialize(context.Background())

		err := rb.Close()
		assert.NoError(t, err)
		assert.True(t, rb.IsClosed())
	})

	t.Run("close with cleanup handler error", func(t *testing.T) {
		rb := NewResourceBase("test-error")
		rb.Initialize(context.Background())

		// Add a failing cleanup handler
		rb.AddCleanHandler(func() error {
			return assert.AnError
		})

		err := rb.Close()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test-error")
		assert.Contains(t, err.Error(), "cleanup failed")
	})

	t.Run("close multiple times", func(t *testing.T) {
		rb := NewResourceBase("test")
		rb.Initialize(context.Background())

		err1 := rb.Close()
		assert.NoError(t, err1)

		err2 := rb.Close()
		assert.NoError(t, err2)
	})
}

func TestResourceBase_GetName(t *testing.T) {
	rb := NewResourceBase("my-resource")
	assert.Equal(t, "my-resource", rb.GetName())
}

func TestResourceBase_SetName(t *testing.T) {
	rb := NewResourceBase("original")
	assert.Equal(t, "original", rb.GetName())

	rb.SetName("updated")
	assert.Equal(t, "updated", rb.GetName())
}

func TestResourceBase_onClose(t *testing.T) {
	rb := NewResourceBase("test")

	// onClose should always return nil
	err := rb.onClose()
	assert.NoError(t, err)
}

// testDisposableResource implements DisposableResource for testing
type testDisposableResource struct {
	*ResourceBase
}

func (t *testDisposableResource) Dispose() error {
	return t.Close()
}

func TestNewDisposableResource(t *testing.T) {
	t.Run("create disposable resource", func(t *testing.T) {
		ctx := context.Background()

		resource := NewDisposableResource("test-resource", ctx, func() *testDisposableResource {
			return &testDisposableResource{
				ResourceBase: NewResourceBase(""),
			}
		})

		assert.NotNil(t, resource)
		assert.Equal(t, "test-resource", resource.GetName())
		assert.False(t, resource.IsClosed())
	})

	t.Run("disposable resource is initialized", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		resource := NewDisposableResource("test", ctx, func() *testDisposableResource {
			return &testDisposableResource{
				ResourceBase: NewResourceBase(""),
			}
		})

		assert.NotNil(t, resource.Ctx())
	})
}

func TestInitializeResource(t *testing.T) {
	t.Run("initialize resource with initializer interface", func(t *testing.T) {
		rb := NewResourceBase("test")
		ctx := context.Background()

		// InitializeResource should call Initialize
		InitializeResource(rb, ctx)

		assert.NotNil(t, rb.ctx)
		assert.False(t, rb.IsClosed())
	})
}

func TestResourceBase_Integration(t *testing.T) {
	// Full lifecycle test
	t.Run("complete resource lifecycle", func(t *testing.T) {
		// Create resource
		rb := NewResourceBase("integration-test")
		assert.Equal(t, "integration-test", rb.GetName())
		assert.False(t, rb.IsClosed())

		// Initialize with context
		ctx := context.Background()
		rb.Initialize(ctx)
		assert.NotNil(t, rb.Ctx())

		// Add cleanup handlers
		cleaned := false
		rb.AddCleanHandler(func() error {
			cleaned = true
			return nil
		})

		// Close resource
		err := rb.Close()
		require.NoError(t, err)
		assert.True(t, rb.IsClosed())
		assert.True(t, cleaned)

		// Verify can close again safely
		err = rb.Close()
		assert.NoError(t, err)
	})

	t.Run("resource with context cancellation", func(t *testing.T) {
		rb := NewResourceBase("cancel-test")
		ctx, cancel := context.WithCancel(context.Background())

		cleaned := false
		rb.Initialize(ctx)
		rb.AddCleanHandler(func() error {
			cleaned = true
			return nil
		})

		// Give goroutine time to start
		time.Sleep(10 * time.Millisecond)

		// Cancel context should trigger cleanup
		cancel()
		time.Sleep(50 * time.Millisecond)

		assert.True(t, rb.IsClosed())
		assert.True(t, cleaned)
	})

	t.Run("resource name mutation", func(t *testing.T) {
		rb := NewResourceBase("original-name")
		assert.Equal(t, "original-name", rb.GetName())

		rb.SetName("new-name")
		assert.Equal(t, "new-name", rb.GetName())

		// Name should appear in error messages
		rb.Initialize(context.Background())
		rb.AddCleanHandler(func() error {
			return assert.AnError
		})

		err := rb.Close()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "new-name")
	})
}

func TestResourceBase_ContextPropagation(t *testing.T) {
	t.Run("context values are accessible", func(t *testing.T) {
		type contextKey string
		key := contextKey("test-key")
		value := "test-value"

		ctx := context.WithValue(context.Background(), key, value)

		rb := NewResourceBase("test")
		rb.Initialize(ctx)

		// Verify context value is accessible
		retrievedValue := rb.Ctx().Value(key)
		assert.Equal(t, value, retrievedValue)
	})

	t.Run("context cancellation propagates", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		rb := NewResourceBase("test")
		rb.Initialize(ctx)

		// Give goroutine time to start
		time.Sleep(10 * time.Millisecond)

		// Cancel parent context
		cancel()

		// Child context should be cancelled
		time.Sleep(50 * time.Millisecond)

		select {
		case <-rb.Ctx().Done():
			// Context was cancelled
		default:
			t.Fatal("Context should be cancelled")
		}
	})
}

func TestResourceBase_CleanupOrder(t *testing.T) {
	rb := NewResourceBase("test")
	rb.Initialize(context.Background())

	var order []int

	// Add multiple cleanup handlers
	rb.AddCleanHandler(func() error {
		order = append(order, 1)
		return nil
	})
	rb.AddCleanHandler(func() error {
		order = append(order, 2)
		return nil
	})
	rb.AddCleanHandler(func() error {
		order = append(order, 3)
		return nil
	})

	err := rb.Close()
	require.NoError(t, err)

	// Handlers should execute in order
	assert.Equal(t, []int{1, 2, 3}, order)
}

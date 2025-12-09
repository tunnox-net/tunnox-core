package dispose

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDisposable for testing
type mockDisposable struct {
	name     string
	disposed bool
	err      error
	mu       sync.Mutex
}

func (m *mockDisposable) Dispose() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disposed = true
	return m.err
}

func (m *mockDisposable) IsDisposed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disposed
}

// disposableFunc adapts a function to Disposable interface
type disposableFunc func() error

func (f disposableFunc) Dispose() error {
	return f()
}

func TestNewResourceManager(t *testing.T) {
	rm := NewResourceManager()
	assert.NotNil(t, rm)
	assert.NotNil(t, rm.resources)
	assert.NotNil(t, rm.order)
	assert.Equal(t, 0, rm.GetResourceCount())
}

func TestResourceManager_Register(t *testing.T) {
	tests := []struct {
		name        string
		resources   []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "register single resource",
			resources:   []string{"res1"},
			expectError: false,
		},
		{
			name:        "register multiple resources",
			resources:   []string{"res1", "res2", "res3"},
			expectError: false,
		},
		{
			name:        "register duplicate resource",
			resources:   []string{"res1", "res1"},
			expectError: true,
			errorMsg:    "already registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := NewResourceManager()

			for i, name := range tt.resources {
				res := &mockDisposable{name: name}
				err := rm.Register(name, res)

				if tt.expectError && i == len(tt.resources)-1 {
					assert.Error(t, err)
					if tt.errorMsg != "" {
						assert.Contains(t, err.Error(), tt.errorMsg)
					}
				} else {
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestResourceManager_Unregister(t *testing.T) {
	t.Run("unregister existing resource", func(t *testing.T) {
		rm := NewResourceManager()
		res := &mockDisposable{name: "test"}

		err := rm.Register("test", res)
		require.NoError(t, err)

		err = rm.Unregister("test")
		assert.NoError(t, err)
		assert.Equal(t, 0, rm.GetResourceCount())
	})

	t.Run("unregister non-existent resource", func(t *testing.T) {
		rm := NewResourceManager()

		err := rm.Unregister("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("unregister from multiple resources", func(t *testing.T) {
		rm := NewResourceManager()

		for i := 0; i < 3; i++ {
			res := &mockDisposable{name: string(rune('a' + i))}
			rm.Register(string(rune('a'+i)), res)
		}

		err := rm.Unregister("b")
		assert.NoError(t, err)
		assert.Equal(t, 2, rm.GetResourceCount())

		list := rm.ListResources()
		assert.Equal(t, []string{"a", "c"}, list)
	})
}

func TestResourceManager_GetResource(t *testing.T) {
	rm := NewResourceManager()
	res1 := &mockDisposable{name: "test"}

	rm.Register("test", res1)

	t.Run("get existing resource", func(t *testing.T) {
		res, exists := rm.GetResource("test")
		assert.True(t, exists)
		assert.Equal(t, res1, res)
	})

	t.Run("get non-existent resource", func(t *testing.T) {
		res, exists := rm.GetResource("nonexistent")
		assert.False(t, exists)
		assert.Nil(t, res)
	})
}

func TestResourceManager_ListResources(t *testing.T) {
	rm := NewResourceManager()

	t.Run("list empty resources", func(t *testing.T) {
		list := rm.ListResources()
		assert.NotNil(t, list)
		assert.Len(t, list, 0)
	})

	t.Run("list multiple resources", func(t *testing.T) {
		names := []string{"res1", "res2", "res3"}
		for _, name := range names {
			rm.Register(name, &mockDisposable{name: name})
		}

		list := rm.ListResources()
		assert.Equal(t, names, list)
	})
}

func TestResourceManager_GetResourceCount(t *testing.T) {
	rm := NewResourceManager()

	assert.Equal(t, 0, rm.GetResourceCount())

	rm.Register("res1", &mockDisposable{})
	assert.Equal(t, 1, rm.GetResourceCount())

	rm.Register("res2", &mockDisposable{})
	assert.Equal(t, 2, rm.GetResourceCount())

	rm.Unregister("res1")
	assert.Equal(t, 1, rm.GetResourceCount())
}

func TestResourceManager_DisposeAll(t *testing.T) {
	t.Run("dispose all resources successfully", func(t *testing.T) {
		rm := NewResourceManager()

		res1 := &mockDisposable{name: "res1"}
		res2 := &mockDisposable{name: "res2"}
		res3 := &mockDisposable{name: "res3"}

		rm.Register("res1", res1)
		rm.Register("res2", res2)
		rm.Register("res3", res3)

		result := rm.DisposeAll()

		assert.False(t, result.HasErrors())
		assert.True(t, res1.IsDisposed())
		assert.True(t, res2.IsDisposed())
		assert.True(t, res3.IsDisposed())
		assert.Equal(t, 0, rm.GetResourceCount())
	})

	t.Run("dispose in reverse order", func(t *testing.T) {
		rm := NewResourceManager()

		var order []string
		var mu sync.Mutex

		// Create custom disposables that track order
		type orderTracker struct {
			name string
			mu   *sync.Mutex
			list *[]string
		}

		for i := 1; i <= 3; i++ {
			name := string(rune('a' + i - 1))
			tracker := &orderTracker{
				name: name,
				mu:   &mu,
				list: &order,
			}

			// Create a closure that captures the order
			rm.Register(name, disposableFunc(func() error {
				tracker.mu.Lock()
				*tracker.list = append(*tracker.list, tracker.name)
				tracker.mu.Unlock()
				return nil
			}))
		}

		rm.DisposeAll()

		// Should dispose in reverse order: c, b, a
		assert.Equal(t, []string{"c", "b", "a"}, order)
	})

	t.Run("dispose with errors", func(t *testing.T) {
		rm := NewResourceManager()

		res1 := &mockDisposable{name: "res1", err: errors.New("error1")}
		res2 := &mockDisposable{name: "res2"}
		res3 := &mockDisposable{name: "res3", err: errors.New("error3")}

		rm.Register("res1", res1)
		rm.Register("res2", res2)
		rm.Register("res3", res3)

		result := rm.DisposeAll()

		assert.True(t, result.HasErrors())
		assert.Len(t, result.Errors, 2)

		// All resources should still be disposed despite errors
		assert.True(t, res1.IsDisposed())
		assert.True(t, res2.IsDisposed())
		assert.True(t, res3.IsDisposed())
	})

	t.Run("dispose empty manager", func(t *testing.T) {
		rm := NewResourceManager()

		result := rm.DisposeAll()
		assert.False(t, result.HasErrors())
		assert.Len(t, result.Errors, 0)
	})

	t.Run("dispose multiple times", func(t *testing.T) {
		rm := NewResourceManager()
		res := &mockDisposable{name: "test"}
		rm.Register("test", res)

		result1 := rm.DisposeAll()
		assert.False(t, result1.HasErrors())

		// Second dispose should return empty result
		result2 := rm.DisposeAll()
		assert.False(t, result2.HasErrors())
		assert.Len(t, result2.Errors, 0)
	})
}

func TestResourceManager_DisposeWithTimeout(t *testing.T) {
	t.Run("dispose completes within timeout", func(t *testing.T) {
		rm := NewResourceManager()
		res := &mockDisposable{name: "fast"}
		rm.Register("fast", res)

		result := rm.DisposeWithTimeout(1 * time.Second)
		assert.False(t, result.HasErrors())
		assert.True(t, res.IsDisposed())
	})

	t.Run("dispose exceeds timeout", func(t *testing.T) {
		rm := NewResourceManager()

		// Create a slow disposable that blocks
		rm.Register("slow", disposableFunc(func() error {
			time.Sleep(2 * time.Second)
			return nil
		}))

		result := rm.DisposeWithTimeout(100 * time.Millisecond)

		assert.True(t, result.HasErrors())
		assert.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0].Error(), "timeout")
	})
}

func TestResourceManager_ConcurrentAccess(t *testing.T) {
	rm := NewResourceManager()

	var wg sync.WaitGroup
	concurrency := 10

	// Concurrent register
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			name := string(rune('a' + idx))
			rm.Register(name, &mockDisposable{name: name})
		}(i)
	}
	wg.Wait()

	assert.Equal(t, concurrency, rm.GetResourceCount())

	// Concurrent read
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			rm.ListResources()
			rm.GetResourceCount()
		}()
	}
	wg.Wait()
}

func TestGlobalResourceManager(t *testing.T) {
	// Clean up before test
	DisposeAllGlobalResources()

	t.Run("register and dispose global resource", func(t *testing.T) {
		res := &mockDisposable{name: "global"}

		err := RegisterGlobalResource("global", res)
		assert.NoError(t, err)

		result := DisposeAllGlobalResources()
		assert.False(t, result.HasErrors())
		assert.True(t, res.IsDisposed())
	})

	t.Run("unregister global resource", func(t *testing.T) {
		res := &mockDisposable{name: "temp"}

		RegisterGlobalResource("temp", res)
		err := UnregisterGlobalResource("temp")
		assert.NoError(t, err)

		result := DisposeAllGlobalResources()
		assert.False(t, result.HasErrors())
		assert.False(t, res.IsDisposed())
	})

	t.Run("dispose global resources with timeout", func(t *testing.T) {
		res := &mockDisposable{name: "timed"}
		RegisterGlobalResource("timed", res)

		result := DisposeAllGlobalResourcesWithTimeout(1 * time.Second)
		assert.False(t, result.HasErrors())
		assert.True(t, res.IsDisposed())
	})
}

func TestIncrementDisposeCount(t *testing.T) {
	t.Run("without count function", func(t *testing.T) {
		// Should not panic
		assert.NotPanics(t, func() {
			IncrementDisposeCount()
		})
	})

	t.Run("with count function", func(t *testing.T) {
		callCount := 0
		SetIncrementDisposeCountFunc(func() {
			callCount++
		})

		IncrementDisposeCount()
		IncrementDisposeCount()

		assert.Equal(t, 2, callCount)

		// Clean up
		SetIncrementDisposeCountFunc(nil)
	})

	t.Run("concurrent increment", func(t *testing.T) {
		var count int
		var mu sync.Mutex

		SetIncrementDisposeCountFunc(func() {
			mu.Lock()
			count++
			mu.Unlock()
		})

		var wg sync.WaitGroup
		iterations := 100
		wg.Add(iterations)

		for i := 0; i < iterations; i++ {
			go func() {
				defer wg.Done()
				IncrementDisposeCount()
			}()
		}

		wg.Wait()

		mu.Lock()
		finalCount := count
		mu.Unlock()

		assert.Equal(t, iterations, finalCount)

		// Clean up
		SetIncrementDisposeCountFunc(nil)
	})
}

func TestResourceManager_DisposingFlag(t *testing.T) {
	rm := NewResourceManager()

	// Create a resource that checks the disposing flag
	res := &mockDisposable{name: "test"}
	rm.Register("test", res)

	// Start dispose in goroutine
	done := make(chan bool)
	go func() {
		rm.DisposeAll()
		done <- true
	}()

	// Wait for completion
	<-done

	// Verify final state
	assert.Equal(t, 0, rm.GetResourceCount())
	assert.False(t, rm.disposing)
}

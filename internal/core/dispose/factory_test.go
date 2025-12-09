package dispose

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewResourceFactory(t *testing.T) {
	factory := NewResourceFactory()
	assert.NotNil(t, factory)
}

func TestResourceFactory_NewManager(t *testing.T) {
	factory := NewResourceFactory()
	ctx := context.Background()

	manager := factory.NewManager("test-manager", ctx)

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.ResourceBase)
	assert.Equal(t, "test-manager", manager.GetName())
	assert.NotNil(t, manager.Ctx())
	assert.False(t, manager.IsClosed())
}

func TestResourceFactory_NewService(t *testing.T) {
	factory := NewResourceFactory()
	ctx := context.Background()

	service := factory.NewService("test-service", ctx)

	assert.NotNil(t, service)
	assert.NotNil(t, service.ResourceBase)
	assert.Equal(t, "test-service", service.GetName())
	assert.NotNil(t, service.Ctx())
	assert.False(t, service.IsClosed())
}

func TestNewManager_GlobalFactory(t *testing.T) {
	ctx := context.Background()

	manager := NewManager("global-manager", ctx)

	assert.NotNil(t, manager)
	assert.Equal(t, "global-manager", manager.GetName())
	assert.NotNil(t, manager.Ctx())
}

func TestNewService_GlobalFactory(t *testing.T) {
	ctx := context.Background()

	service := NewService("global-service", ctx)

	assert.NotNil(t, service)
	assert.Equal(t, "global-service", service.GetName())
	assert.NotNil(t, service.Ctx())
}

func TestGlobalResourceFactory(t *testing.T) {
	assert.NotNil(t, GlobalResourceFactory)
}

func TestManagerBase_Lifecycle(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("lifecycle-test", ctx)

	// Test initial state
	assert.Equal(t, "lifecycle-test", manager.GetName())
	assert.False(t, manager.IsClosed())

	// Test cleanup handler
	cleaned := false
	manager.AddCleanHandler(func() error {
		cleaned = true
		return nil
	})

	// Close manager
	err := manager.Close()
	assert.NoError(t, err)
	assert.True(t, manager.IsClosed())
	assert.True(t, cleaned)
}

func TestServiceBase_Lifecycle(t *testing.T) {
	ctx := context.Background()
	service := NewService("service-lifecycle-test", ctx)

	// Test initial state
	assert.Equal(t, "service-lifecycle-test", service.GetName())
	assert.False(t, service.IsClosed())

	// Test cleanup handler
	cleaned := false
	service.AddCleanHandler(func() error {
		cleaned = true
		return nil
	})

	// Close service
	err := service.Close()
	assert.NoError(t, err)
	assert.True(t, service.IsClosed())
	assert.True(t, cleaned)
}

func TestManagerBase_SetName(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("original-manager", ctx)

	assert.Equal(t, "original-manager", manager.GetName())

	manager.SetName("renamed-manager")
	assert.Equal(t, "renamed-manager", manager.GetName())
}

func TestServiceBase_SetName(t *testing.T) {
	ctx := context.Background()
	service := NewService("original-service", ctx)

	assert.Equal(t, "original-service", service.GetName())

	service.SetName("renamed-service")
	assert.Equal(t, "renamed-service", service.GetName())
}

func TestFactory_MultipleResources(t *testing.T) {
	factory := NewResourceFactory()
	ctx := context.Background()

	// Create multiple managers
	manager1 := factory.NewManager("mgr1", ctx)
	manager2 := factory.NewManager("mgr2", ctx)

	// Create multiple services
	service1 := factory.NewService("svc1", ctx)
	service2 := factory.NewService("svc2", ctx)

	assert.NotEqual(t, manager1, manager2)
	assert.NotEqual(t, service1, service2)

	assert.Equal(t, "mgr1", manager1.GetName())
	assert.Equal(t, "mgr2", manager2.GetName())
	assert.Equal(t, "svc1", service1.GetName())
	assert.Equal(t, "svc2", service2.GetName())
}

func TestFactory_ContextPropagation(t *testing.T) {
	factory := NewResourceFactory()

	type contextKey string
	key := contextKey("test-key")
	value := "test-value"

	ctx := context.WithValue(context.Background(), key, value)

	manager := factory.NewManager("ctx-test", ctx)

	// Verify context value is accessible
	retrievedValue := manager.Ctx().Value(key)
	assert.Equal(t, value, retrievedValue)
}

func TestFactory_Integration(t *testing.T) {
	// Test complete factory pattern integration
	factory := NewResourceFactory()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create manager
	manager := factory.NewManager("integration-manager", ctx)
	assert.NotNil(t, manager)
	assert.Equal(t, "integration-manager", manager.GetName())
	assert.False(t, manager.IsClosed())

	// Create service
	service := factory.NewService("integration-service", ctx)
	assert.NotNil(t, service)
	assert.Equal(t, "integration-service", service.GetName())
	assert.False(t, service.IsClosed())

	// Add cleanup handlers to verify they're called
	managerCleaned := false
	manager.AddCleanHandler(func() error {
		managerCleaned = true
		return nil
	})

	serviceCleaned := false
	service.AddCleanHandler(func() error {
		serviceCleaned = true
		return nil
	})

	// Close both resources
	err := manager.Close()
	assert.NoError(t, err)
	assert.True(t, manager.IsClosed())
	assert.True(t, managerCleaned)

	err = service.Close()
	assert.NoError(t, err)
	assert.True(t, service.IsClosed())
	assert.True(t, serviceCleaned)
}

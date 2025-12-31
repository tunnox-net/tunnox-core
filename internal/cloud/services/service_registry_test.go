package services

import (
	"context"
	"testing"

	"tunnox-core/internal/cloud/container"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServiceRegistry(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	defer c.Close()

	registry := NewServiceRegistry(c)
	require.NotNil(t, registry)
}

func TestServiceRegistry_TypeAlias(t *testing.T) {
	// 测试类型别名是否正确工作
	ctx := context.Background()
	c := container.NewContainer(ctx)
	defer c.Close()

	var _ *ServiceRegistry = NewServiceRegistry(c)
	assert.True(t, true, "Type alias works correctly")
}

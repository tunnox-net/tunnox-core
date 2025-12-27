package factories

import (
	"context"
	"fmt"
	"tunnox-core/internal/cloud/managers"
)

// NewCloudControlAPI 创建云控API实例
func NewCloudControlAPI(parentCtx context.Context, config *managers.ControlConfig) (managers.CloudControlAPI, error) {
	if config == nil {
		config = managers.DefaultConfig()
	}

	if config.UseBuiltIn {
		return NewBuiltinCloudControlAPI(parentCtx, config)
	}

	return nil, fmt.Errorf("REST API not implemented yet")
}

// NewBuiltinCloudControlAPI 创建内置云控实例
func NewBuiltinCloudControlAPI(parentCtx context.Context, config *managers.ControlConfig) (managers.CloudControlAPI, error) {
	if config == nil {
		config = managers.DefaultConfig()
	}

	builtin := managers.NewBuiltinCloudControl(parentCtx, config)
	builtin.Start()

	return builtin, nil
}

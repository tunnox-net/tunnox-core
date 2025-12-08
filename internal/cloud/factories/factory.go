package factories

import (
	"context"
	"tunnox-core/internal/cloud/managers"
	coreErrors "tunnox-core/internal/core/errors"
)

// NewCloudControlAPI 创建云控API实例
func NewCloudControlAPI(config *managers.ControlConfig, parentCtx context.Context) (managers.CloudControlAPI, error) {
	if config == nil {
		config = managers.DefaultConfig()
	}

	if config.UseBuiltIn {
		return NewBuiltinCloudControlAPI(config, parentCtx)
	}

	return nil, coreErrors.New(coreErrors.ErrorTypePermanent, "REST API not implemented yet")
}

// NewBuiltinCloudControlAPI 创建内置云控实例
func NewBuiltinCloudControlAPI(config *managers.ControlConfig, parentCtx context.Context) (managers.CloudControlAPI, error) {
	if config == nil {
		config = managers.DefaultConfig()
	}

	builtin := managers.NewBuiltinCloudControl(config, parentCtx)
	builtin.Start()

	return builtin, nil
}

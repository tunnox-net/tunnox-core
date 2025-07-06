package cloud

import (
	"fmt"
)

// NewCloudControlAPI 创建云控API实例
func NewCloudControlAPI(config *ControlConfig) (CloudControlAPI, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.UseBuiltIn {
		return NewBuiltinCloudControlAPI(config)
	}

	return nil, fmt.Errorf("REST API not implemented yet")
}

// NewBuiltinCloudControlAPI 创建内置云控实例
func NewBuiltinCloudControlAPI(config *ControlConfig) (CloudControlAPI, error) {
	if config == nil {
		config = DefaultConfig()
	}

	builtin := NewBuiltinCloudControl(config)
	builtin.Start()

	return builtin, nil
}

package cloud

import (
	"fmt"
)

// NewCloudControlAPI 创建云控API实例
func NewCloudControlAPI(config *CloudControlConfig) (CloudControlAPI, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.UseBuiltIn {
		return NewBuiltinCloudControl(config)
	}

	return nil, fmt.Errorf("REST API not implemented yet")
}

// NewBuiltinCloudControl 创建内置云控实例
func NewBuiltinCloudControl(config *CloudControlConfig) (CloudControlAPI, error) {
	if config == nil {
		config = DefaultConfig()
	}

	builtin := NewBuiltInCloudControl(config)
	builtin.Start()

	return builtin, nil
}

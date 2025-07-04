package cloud

import "fmt"

// NewCloudControlAPI 创建云控API实例
func NewCloudControlAPI(config *CloudControlConfig) (CloudControlAPI, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.UseBuiltIn {
		return nil, fmt.Errorf("built-in API not implemented yet")
	}

	return nil, fmt.Errorf("REST API not implemented yet")
}

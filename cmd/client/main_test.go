package main

import (
	"testing"

	"tunnox-core/internal/client"
)

// TestLoadOrCreateConfig_AutoConnectConditions 测试自动连接条件
func TestLoadOrCreateConfig_AutoConnectConditions(t *testing.T) {
	tests := []struct {
		name           string
		configFile     string
		serverAddr     string
		isCLIMode      bool
		expectAutoConn bool
	}{
		{
			name:           "CLI模式_无配置文件_无命令行地址_应该自动连接",
			configFile:     "",
			serverAddr:     "",
			isCLIMode:      true,
			expectAutoConn: true,
		},
		{
			name:           "CLI模式_有命令行地址_不应该自动连接",
			configFile:     "",
			serverAddr:     "localhost:8000",
			isCLIMode:      true,
			expectAutoConn: false,
		},
		{
			name:           "Daemon模式_无配置文件_无命令行地址_不应该自动连接",
			configFile:     "",
			serverAddr:     "",
			isCLIMode:      false,
			expectAutoConn: false,
		},
		{
			name:           "CLI模式_配置文件有地址_不应该自动连接",
			configFile:     "",
			serverAddr:     "",
			isCLIMode:      true,
			expectAutoConn: true, // 因为 getDefaultConfig 不再设置默认地址
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := loadOrCreateConfig(tt.configFile, "", tt.serverAddr, 0, "", "", false, tt.isCLIMode)
			if err != nil {
				t.Fatalf("loadOrCreateConfig failed: %v", err)
			}

			// 检查是否符合自动连接条件
			needsAutoConnect := tt.isCLIMode && config.Server.Address == "" && tt.serverAddr == ""
			if needsAutoConnect != tt.expectAutoConn {
				t.Errorf("Expected autoConnect=%v, got %v (Address=%q, serverAddr=%q, isCLIMode=%v)",
					tt.expectAutoConn, needsAutoConnect, config.Server.Address, tt.serverAddr, tt.isCLIMode)
			}
		})
	}
}

// TestValidateConfig_AutoConnectMode 测试自动连接模式下的配置验证
func TestValidateConfig_AutoConnectMode(t *testing.T) {
	config := &client.ClientConfig{
		Anonymous: true,
		DeviceID:  "test-device",
		// Address 和 Protocol 都为空，模拟自动连接模式
	}

	// 自动连接模式（setDefaults=false）
	err := validateConfig(config, false)
	if err != nil {
		t.Fatalf("validateConfig should not fail in auto-connect mode: %v", err)
	}

	// 验证地址和协议保持为空
	if config.Server.Address != "" {
		t.Errorf("Expected empty address in auto-connect mode, got %q", config.Server.Address)
	}
}

// TestValidateConfig_NonAutoConnectMode 测试非自动连接模式下的配置验证
func TestValidateConfig_NonAutoConnectMode(t *testing.T) {
	config := &client.ClientConfig{
		Anonymous: true,
		DeviceID:  "test-device",
		// Address 和 Protocol 都为空
	}

	// 非自动连接模式（setDefaults=true），应该设置默认值
	err := validateConfig(config, true)
	if err != nil {
		t.Fatalf("validateConfig failed: %v", err)
	}

	// 验证设置了默认地址和协议
	if config.Server.Address == "" {
		t.Error("Expected default address to be set in non-auto-connect mode")
	}
	if config.Server.Protocol == "" {
		t.Error("Expected default protocol to be set in non-auto-connect mode")
	}
}

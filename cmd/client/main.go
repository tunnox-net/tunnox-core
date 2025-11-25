package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tunnox-core/internal/client"
	"tunnox-core/internal/utils"
	"gopkg.in/yaml.v3"
)

func main() {
	// 解析命令行参数
	configFile := flag.String("config", "client-config.yaml", "path to config file")
	flag.Parse()

	// 加载配置
	config, err := loadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建客户端
	tunnoxClient := client.NewClient(ctx, config)

	// 连接到服务器
	if err := tunnoxClient.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to server: %v\n", err)
		os.Exit(1)
	}

	utils.Infof("Client: successfully connected to server")

	// 等待终止信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		utils.Infof("Client: received signal %v, shutting down...", sig)
	case <-ctx.Done():
		utils.Infof("Client: context cancelled, shutting down...")
	}

	// 停止客户端
	tunnoxClient.Stop()
	utils.Infof("Client: shutdown complete")
}

// loadConfig 加载配置文件
func loadConfig(path string) (*client.ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config client.ClientConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

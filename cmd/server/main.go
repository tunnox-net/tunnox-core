package main

import (
	"context"
	"flag"
	"path/filepath"
	"tunnox-core/internal/app/server"
	"tunnox-core/internal/utils"
)

func main() {
	// 1. 解析命令行参数
	var (
		configPath = flag.String("config", "config.yaml", "Path to configuration file")
		showHelp   = flag.Bool("help", false, "Show help information")
	)
	flag.Parse()

	// 显示帮助信息
	if *showHelp {
		utils.Info("Tunnox Core Server")
		utils.Info("Usage: server [options]")
		utils.Info()
		utils.Info("Options:")
		flag.PrintDefaults()
		utils.Info()
		utils.Info("Examples:")
		utils.Info("  server                    # 使用当前目录下的 config.yaml")
		utils.Info("  server -config ./my_config.yaml")
		utils.Info("  server -config /path/to/config.yaml")
		return
	}

	// 获取配置文件绝对路径
	absConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		utils.Fatalf("Failed to resolve config path: %v", err)
	}

	// 2. 加载配置并创建服务器
	config, err := server.LoadConfig(absConfigPath)
	if err != nil {
		utils.Fatalf("Failed to load configuration: %v", err)
	}

	srv := server.New(config, context.Background())

	// 显示启动信息横幅（在日志初始化之后，服务启动之前）
	srv.DisplayStartupBanner(absConfigPath)

	// 3. 运行服务器（包含信号处理和优雅关闭）
	if err := srv.Run(); err != nil {
		utils.Fatalf("Failed to run server: %v", err)
	}

	utils.Info("Tunnox Core server exited gracefully")
}

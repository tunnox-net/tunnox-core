package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"tunnox-core/internal/app/server"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils"
)

func main() {
	// 1. 解析命令行参数
	var (
		configPath = flag.String("config", "config.yaml", "Path to configuration file")
		logFile    = flag.String("log", "", "Log file path (overrides config file)")
		showHelp   = flag.Bool("help", false, "Show help information")
	)
	flag.Parse()

	// 显示帮助信息
	if *showHelp {
		corelog.Info("Tunnox Core Server")
		corelog.Info("Usage: server [options]")
		corelog.Info()
		corelog.Info("Options:")
		flag.PrintDefaults()
		corelog.Info()
		corelog.Info("Examples:")
		corelog.Info("  server                    # 使用当前目录下的 config.yaml")
		corelog.Info("  server -config ./my_config.yaml")
		corelog.Info("  server -config /path/to/config.yaml -log /tmp/server.log")
		corelog.Info("  server -log /var/log/tunnox/server.log")
		return
	}

	// 获取配置文件绝对路径
	absConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		corelog.Fatalf("Failed to resolve config path: %v", err)
	}

	// 2. 加载配置并创建服务器
	config, err := server.LoadConfig(absConfigPath)
	if err != nil {
		corelog.Fatalf("Failed to load configuration: %v", err)
	}

	// 3. 如果指定了日志文件路径，覆盖配置
	if *logFile != "" {
		expandedPath, err := utils.ExpandPath(*logFile)
		if err != nil {
			corelog.Fatalf("Failed to expand log file path %q: %v", *logFile, err)
		}
		config.Log.File = expandedPath
		config.Log.Output = "file"
		// 确保日志目录存在
		logDir := filepath.Dir(expandedPath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			corelog.Fatalf("Failed to create log directory %q: %v", logDir, err)
		}
	}

	srv := server.New(config, context.Background())

	// 显示启动信息横幅（在日志初始化之后，服务启动之前）
	srv.DisplayStartupBanner(absConfigPath)

	// 3. 运行服务器（包含信号处理和优雅关闭）
	if err := srv.Run(); err != nil {
		// 确保错误信息输出到控制台（即使日志配置为只输出到文件）
		fmt.Fprintf(os.Stderr, "ERROR: Failed to run server: %v\n", err)
		os.Exit(1)
	}

	corelog.Info("Tunnox Core server exited gracefully")
}

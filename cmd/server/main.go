package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"tunnox-core/internal/app/server"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils"
)

func main() {
	// 添加全局 panic 恢复机制
	defer func() {
		if r := recover(); r != nil {
			// 获取堆栈信息
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			stackTrace := string(buf[:n])

			// 记录到日志
			corelog.Errorf("FATAL: Server panic recovered: %v\nStack trace:\n%s", r, stackTrace)

			// 同时输出到 stderr，确保即使日志系统失败也能看到
			fmt.Fprintf(os.Stderr, "\n=== FATAL ERROR ===\n")
			fmt.Fprintf(os.Stderr, "Server crashed with panic: %v\n", r)
			fmt.Fprintf(os.Stderr, "\nStack trace:\n%s\n", stackTrace)
			fmt.Fprintf(os.Stderr, "==================\n\n")

			os.Exit(1)
		}
	}()

	// 1. 解析命令行参数
	var (
		configPath   = flag.String("config", "config.yaml", "Path to configuration file")
		logFile      = flag.String("log", "", "Log file path (overrides config file)")
		exportConfig = flag.String("export-config", "", "Export configuration template to file and exit")
		showHelp     = flag.Bool("help", false, "Show help information")
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
		corelog.Info("  server                              # 使用当前目录下的 config.yaml")
		corelog.Info("  server -config ./my_config.yaml     # 使用指定配置文件")
		corelog.Info("  server -export-config config.yaml   # 导出配置模板到文件")
		corelog.Info("  server -log /var/log/tunnox.log     # 指定日志文件")
		return
	}

	// 导出配置模板
	if *exportConfig != "" {
		if err := server.ExportConfigTemplate(*exportConfig); err != nil {
			corelog.Fatalf("Failed to export config template: %v", err)
		}
		corelog.Infof("Configuration template exported to: %s", *exportConfig)
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
		// 确保日志目录存在
		logDir := filepath.Dir(expandedPath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			corelog.Fatalf("Failed to create log directory %q: %v", logDir, err)
		}
	}

	// 4. 初始化日志系统（服务端固定为 console+file）
	logConfig := &utils.LogConfig{
		Level:  config.Log.Level,
		Format: "text",
		Output: "both", // 服务端固定同时输出到 console 和 file
		File:   config.Log.File,
	}
	if err := utils.InitLogger(logConfig); err != nil {
		corelog.Fatalf("Failed to initialize logger: %v", err)
	}

	srv := server.New(config, context.Background())

	// 显示启动信息横幅（在日志初始化之后，服务启动之前）
	srv.DisplayStartupBanner(absConfigPath)

	// 记录启动开始（带明显标记，便于在混合日志中识别）
	corelog.Infof("========================================")
	corelog.Infof("🚀 SERVER STARTING - PID: %d", os.Getpid())
	corelog.Infof("========================================")

	// 5. 运行服务器（包含信号处理和优雅关闭）
	if err := srv.Run(); err != nil {
		// 确保错误信息输出到控制台（即使日志配置为只输出到文件）
		fmt.Fprintf(os.Stderr, "ERROR: Failed to run server: %v\n", err)
		os.Exit(1)
	}

	corelog.Info("Tunnox Core server exited gracefully")
	corelog.Infof("========================================")
	corelog.Infof("✅ SERVER STOPPED - PID: %d", os.Getpid())
	corelog.Infof("========================================")
}

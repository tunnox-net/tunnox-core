package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"tunnox-core/internal/version"

	"github.com/fatih/color"
)

const (
	bannerWidth = 60
)

var (
	bannerCyan    = color.New(color.FgCyan).SprintFunc()
	bannerBlue    = color.New(color.FgBlue).SprintFunc()
	bannerMagenta = color.New(color.FgMagenta).SprintFunc()
	bannerBold    = color.New(color.Bold).SprintFunc()
	bannerGreen   = color.New(color.FgGreen).SprintFunc()
	bannerFaint   = color.New(color.Faint).SprintFunc()
)

// DisplayStartupBanner 显示启动信息横幅
func (s *Server) DisplayStartupBanner(configPath string) {
	clearScreen()
	reset := color.New(color.Reset).SprintFunc()

	displayLogo(reset)
	displayServerInfo(s, configPath, reset)
	displayProtocolListeners(s, reset)
	displayManagementAPI(s, reset)
	displayFooter(reset)
}

// clearScreen 清屏
func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

// displayLogo 显示 Logo
func displayLogo(reset func(...interface{}) string) {
	fmt.Println()
	fmt.Printf("  %s_____ _   _ _   _ _   _  _____  __%s\n", bannerCyan(""), reset(""))
	fmt.Printf("  %s|_   _| | | | \\ | | \\ | |/ _ \\ \\/ /%s    %s%sTunnox Core Server%s\n",
		bannerCyan(""), reset(""), bannerFaint(""), bannerBold(""), reset(""))
	fmt.Printf("  %s  | | | | | |  \\| |  \\| | | | \\  /%s\n", bannerBlue(""), reset(""))
	fmt.Printf("  %s  | | | |_| | |\\  | |\\  | |_| /  \\%s     %sVersion %s%s\n",
		bannerBlue(""), reset(""), bannerFaint(""), version.GetShortVersion(), reset(""))
	fmt.Printf("  %s  |_|  \\___/|_| \\_|_| \\_|\\___/_/\\_\\%s\n", bannerMagenta(""), reset(""))
	fmt.Println()
}

// displayServerInfo 显示服务器信息
func displayServerInfo(s *Server, configPath string, reset func(...interface{}) string) {
	fmt.Println(bannerBold("  Server Information"))
	fmt.Println(bannerFaint("  " + strings.Repeat("─", bannerWidth)))

	logFile := getLogFilePath(s.config.Log.File)
	runMode := getRunMode(s.config)
	cacheInfo := formatCacheInfo(s.config)
	persistentInfo := formatPersistentStorageInfo(s.config)

	infoRows := []struct {
		label string
		value string
	}{
		{"Node ID", s.nodeID},
		{"Config File", configPath},
		{"Start Time", time.Now().Format("2006-01-02 15:04:05")},
		{"Run Mode", runMode},
		{"Cache", cacheInfo},
		{"Persistent", persistentInfo},
		{"Log File", logFile},
	}

	for _, row := range infoRows {
		fmt.Printf("  %-18s %s\n", bannerBold(row.label+":"), row.value)
	}
	fmt.Println()
}

// displayProtocolListeners 显示协议监听状态
func displayProtocolListeners(s *Server, reset func(...interface{}) string) {
	fmt.Println(bannerBold("  Protocol Listeners"))
	fmt.Println(bannerFaint("  " + strings.Repeat("─", bannerWidth)))

	// 只显示独立端口的协议（TCP, KCP, QUIC）
	protocolNames := []string{"tcp", "kcp", "quic"}
	for _, name := range protocolNames {
		cfg, exists := s.config.Server.Protocols[name]
		if !exists {
			continue
		}

		displayName := strings.ToUpper(name[:1]) + name[1:]
		if name == "tcp" {
			displayName = "TCP"
		} else if name == "kcp" {
			displayName = "KCP"
		} else if name == "quic" {
			displayName = "QUIC"
		}

		status := bannerFaint("✗ Disabled")
		addr := ""
		if cfg.Enabled {
			status = bannerGreen("✓ Enabled")
			addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		}
		fmt.Printf("  %-12s %-20s %s\n", displayName+":", addr, status)
	}
	fmt.Println()
}

// displayManagementAPI 显示HTTP服务信息（包含所有HTTP模块）
func displayManagementAPI(s *Server, reset func(...interface{}) string) {
	fmt.Println(bannerBold("  HTTP Service"))
	fmt.Println(bannerFaint("  " + strings.Repeat("─", bannerWidth)))

	authType := s.config.Management.Auth.Type
	if authType == "" {
		authType = "none"
	}

	fmt.Printf("  %-18s %s\n", bannerBold("Status:"), bannerGreen("✓ Enabled"))
	fmt.Printf("  %-18s %s\n", bannerBold("Address:"), fmt.Sprintf("http://%s", s.config.Management.Listen))
	fmt.Printf("  %-18s %s\n", bannerBold("Authentication:"), authType)
	fmt.Printf("  %-18s %s\n", bannerBold("Base Path:"), bannerFaint("/tunnox/v1"))
	fmt.Println()

	// 显示已启用的模块
	fmt.Printf("  %s\n", bannerBold("Modules:"))
	fmt.Printf("    • %s\n", "Management API")

	// 检查 WebSocket 是否启用
	if wsConfig, exists := s.config.Server.Protocols["websocket"]; exists && wsConfig.Enabled {
		fmt.Printf("    • %s %s\n", "WebSocket", bannerFaint("(ws://"+s.config.Management.Listen+"/_tunnox)"))
	}

	if s.config.Management.PProf.Enabled {
		fmt.Printf("    • %s %s\n", "PProf", bannerFaint("(/tunnox/v1/debug/pprof/)"))
	}
	fmt.Println()
}

// displayFooter 显示页脚
func displayFooter(reset func(...interface{}) string) {
	fmt.Println(bannerFaint("  " + strings.Repeat("━", bannerWidth)))
	fmt.Println()
	fmt.Printf("  %sServer is starting...%s\n", bannerFaint(""), reset(""))
}

// getLogFilePath 获取日志文件路径
func getLogFilePath(configuredPath string) string {
	logFile := configuredPath
	if logFile == "" {
		logFile = "logs/server.log"
	}
	expandedPath, err := filepath.Abs(logFile)
	if err != nil {
		return logFile
	}
	return expandedPath
}

// getRunMode 获取运行模式
func getRunMode(config *Config) string {
	if config.Storage.Enabled {
		return "Remote Storage"
	}
	if config.Redis.Enabled {
		return "Cluster (Redis)"
	}
	if config.Persistence.Enabled {
		return "Standalone (Persistent)"
	}
	return "Standalone (Memory)"
}

// formatCacheInfo 格式化缓存信息
func formatCacheInfo(config *Config) string {
	if config.Redis.Enabled {
		return fmt.Sprintf("Redis (%s)", config.Redis.Addr)
	}
	return "Memory"
}

// formatPersistentStorageInfo 格式化持久化存储信息
func formatPersistentStorageInfo(config *Config) string {
	if config.Storage.Enabled {
		return fmt.Sprintf("Remote (%s)", config.Storage.URL)
	}
	if config.Persistence.Enabled {
		return fmt.Sprintf("Local (%s)", config.Persistence.File)
	}
	return "None"
}

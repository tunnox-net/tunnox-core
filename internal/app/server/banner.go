package server

import (
	"fmt"
	"path/filepath"
	"sort"
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
	displayServerInfo(s, configPath)
	displayProtocolListeners(s)
	displayManagementAPI(s)
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
func displayServerInfo(s *Server, configPath string) {
	fmt.Println(bannerBold("  Server Information"))
	fmt.Println(bannerFaint("  " + strings.Repeat("─", bannerWidth)))

	logFile := getLogFilePath(s.config.Log.File)
	storageInfo := formatStorageInfo(s.config.Storage)
	brokerInfo := formatBrokerInfo(s.config.MessageBroker)

	infoRows := []struct {
		label string
		value string
	}{
		{"Node ID", s.nodeID},
		{"Config File", configPath},
		{"Start Time", time.Now().Format("2006-01-02 15:04:05")},
		{"Storage", storageInfo},
		{"Message Broker", brokerInfo},
		{"Log File", logFile},
	}

	for _, row := range infoRows {
		fmt.Printf("  %-18s %s\n", bannerBold(row.label+":"), row.value)
	}
	fmt.Println()
}

// displayProtocolListeners 显示协议监听状态
func displayProtocolListeners(s *Server) {
	fmt.Println(bannerBold("  Protocol Listeners"))
	fmt.Println(bannerFaint("  " + strings.Repeat("─", bannerWidth)))

	// 从配置中动态获取协议列表，而不是硬编码（符合可插拔原则）
	// 按字母顺序排序，确保显示一致
	protocolNames := make([]string, 0, len(s.config.Server.Protocols))
	for name := range s.config.Server.Protocols {
		protocolNames = append(protocolNames, name)
	}
	sort.Strings(protocolNames)

	for _, name := range protocolNames {
		cfg := s.config.Server.Protocols[name]

		// 动态生成显示名称（首字母大写，特殊处理）
		displayName := formatProtocolDisplayName(name)

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

// formatProtocolDisplayName 格式化协议显示名称（可插拔，不硬编码）
func formatProtocolDisplayName(name string) string {
	// 特殊处理常见的协议名称
	switch name {
	case "websocket":
		return "WebSocket"
	case "httppoll":
		return "HTTP Poll"
	case "tcp":
		return "TCP"
	case "udp":
		return "UDP"
	case "quic":
		return "QUIC"
	default:
		// 默认：首字母大写
		if len(name) == 0 {
			return name
		}
		return strings.ToUpper(name[:1]) + name[1:]
	}
}

// displayManagementAPI 显示管理 API 信息
func displayManagementAPI(s *Server) {
	if !s.config.ManagementAPI.Enabled {
		return
	}

	fmt.Println(bannerBold("  Management API"))
	fmt.Println(bannerFaint("  " + strings.Repeat("─", bannerWidth)))

	authType := s.config.ManagementAPI.Auth.Type
	if authType == "" {
		authType = "none"
	}

	fmt.Printf("  %-18s %s\n", bannerBold("Status:"), bannerGreen("✓ Enabled"))
	fmt.Printf("  %-18s %s\n", bannerBold("Address:"), fmt.Sprintf("http://%s", s.config.ManagementAPI.ListenAddr))
	fmt.Printf("  %-18s %s\n", bannerBold("Authentication:"), authType)
	fmt.Printf("  %-18s %s\n", bannerBold("Base Path:"), bannerFaint("/tunnox/v1"))

	// 动态检查是否启用了 HTTP Poll 协议（符合可插拔原则）
	if httppollCfg, exists := s.config.Server.Protocols["httppoll"]; exists && httppollCfg.Enabled {
		fmt.Printf("  %-18s %s\n", bannerBold("HTTP Long Poll:"), bannerFaint("POST /tunnox/v1/push, GET /tunnox/v1/poll"))
	}

	if s.config.ManagementAPI.PProf.Enabled {
		fmt.Printf("  %-18s %s\n", bannerBold("PProf:"), bannerFaint("/tunnox/v1/debug/pprof/"))
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

// formatStorageInfo 格式化存储信息
func formatStorageInfo(storage StorageConfig) string {
	switch storage.Type {
	case "hybrid":
		if storage.Redis.Addr != "" {
			return fmt.Sprintf("Hybrid (Memory + Redis: %s)", storage.Redis.Addr)
		}
		return "Hybrid (Memory + JSON)"
	case "redis":
		if storage.Redis.Addr != "" {
			return fmt.Sprintf("Redis (%s)", storage.Redis.Addr)
		}
		return "Redis"
	case "memory":
		return "Memory"
	default:
		return storage.Type
	}
}

// formatBrokerInfo 格式化消息代理信息
func formatBrokerInfo(broker MessageBrokerConfig) string {
	if broker.Type == "redis" && broker.Redis.Addr != "" {
		return fmt.Sprintf("Redis (%s)", broker.Redis.Addr)
	}
	return broker.Type
}

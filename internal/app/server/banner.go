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
	cacheInfo := formatCacheInfo(s.config.Storage)
	persistentInfo := formatPersistentStorageInfo(s.config.Storage)
	brokerInfo := formatBrokerInfo(s.config.MessageBroker)

	infoRows := []struct {
		label string
		value string
	}{
		{"Node ID", s.nodeID},
		{"Config File", configPath},
		{"Start Time", time.Now().Format("2006-01-02 15:04:05")},
		{"Cache", cacheInfo},
		{"Persistent", persistentInfo},
		{"Message Broker", brokerInfo},
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

	protocolNames := []string{"tcp", "websocket", "kcp", "quic", "httppoll"}
	for _, name := range protocolNames {
		cfg, exists := s.config.Server.Protocols[name]
		if !exists {
			continue
		}

		displayName := strings.ToUpper(name[:1]) + name[1:]
		if name == "websocket" {
			displayName = "WebSocket"
		} else if name == "tcp" {
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

// displayManagementAPI 显示管理 API 信息
func displayManagementAPI(s *Server, reset func(...interface{}) string) {
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
	fmt.Printf("  %-18s %s\n", bannerBold("HTTP Long Poll:"), bannerFaint("POST /tunnox/v1/push, GET /tunnox/v1/poll"))
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

// formatCacheInfo 格式化缓存信息
func formatCacheInfo(storage StorageConfig) string {
	switch storage.Type {
	case "hybrid":
		if storage.Hybrid.CacheType == "redis" && storage.Redis.Addr != "" {
			return fmt.Sprintf("Redis (%s)", storage.Redis.Addr)
		}
		return "Memory"
	case "redis":
		if storage.Redis.Addr != "" {
			return fmt.Sprintf("Redis (%s)", storage.Redis.Addr)
		}
		return "Redis"
	case "memory":
		return "Memory"
	default:
		return "Memory"
	}
}

// formatPersistentStorageInfo 格式化持久化存储信息
func formatPersistentStorageInfo(storage StorageConfig) string {
	switch storage.Type {
	case "hybrid":
		if storage.Hybrid.EnablePersistent {
			if storage.Hybrid.Remote.Type == "grpc" && storage.Hybrid.Remote.GRPC.Address != "" {
				return fmt.Sprintf("Remote gRPC (%s)", storage.Hybrid.Remote.GRPC.Address)
			}
			return "Local JSON"
		}
		return "None"
	case "redis":
		return "Redis (built-in)"
	case "memory":
		return "None"
	default:
		return "None"
	}
}

// formatBrokerInfo 格式化消息代理信息
func formatBrokerInfo(broker MessageBrokerConfig) string {
	if broker.Type == "redis" && broker.Redis.Addr != "" {
		return fmt.Sprintf("Redis (%s)", broker.Redis.Addr)
	}
	return broker.Type
}

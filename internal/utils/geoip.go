package utils

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GeoIPResult IP 地理位置查询结果
type GeoIPResult struct {
	IP       string `json:"ip"`
	Country  string `json:"country"`
	Region   string `json:"regionName"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
	Success  bool   `json:"status"`
	ErrorMsg string `json:"message"`
}

// 缓存 IP 地区信息，避免重复查询
var (
	geoIPCache     = make(map[string]string)
	geoIPCacheMu   sync.RWMutex
	geoIPCacheSize = 1000 // 最大缓存数量
)

// LookupIPRegion 查询 IP 所在地区
// 返回格式如 "广州" 或 "中国广州"
func LookupIPRegion(ip string) string {
	if ip == "" {
		return ""
	}

	// 检查是否是内网 IP
	if isPrivateIP(ip) {
		return "内网"
	}

	// 先查缓存
	geoIPCacheMu.RLock()
	if region, ok := geoIPCache[ip]; ok {
		geoIPCacheMu.RUnlock()
		return region
	}
	geoIPCacheMu.RUnlock()

	// 查询外部 API
	region := queryGeoIP(ip)

	// 写入缓存
	geoIPCacheMu.Lock()
	if len(geoIPCache) >= geoIPCacheSize {
		// 简单清理：清空缓存
		geoIPCache = make(map[string]string)
	}
	geoIPCache[ip] = region
	geoIPCacheMu.Unlock()

	return region
}

// queryGeoIP 调用外部 API 查询 IP 地区
func queryGeoIP(ip string) string {
	// 使用 ip-api.com 免费 API (限制 45 请求/分钟)
	// 中文结果：添加 lang=zh-CN 参数
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,regionName,city&lang=zh-CN", ip)

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var result GeoIPResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	if result.Success || result.ErrorMsg == "" {
		// 构建地区字符串
		// 优先显示城市，没有城市显示地区，都没有显示国家
		if result.City != "" {
			return result.City
		}
		if result.Region != "" {
			return result.Region
		}
		if result.Country != "" {
			return result.Country
		}
	}

	return ""
}

// isPrivateIP 判断是否是内网 IP
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// IPv4 私有地址范围
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"100.64.0.0/10", // CGNAT
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	// IPv6 本地地址
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}

	return false
}

// ClearGeoIPCache 清除 GeoIP 缓存
func ClearGeoIPCache() {
	geoIPCacheMu.Lock()
	geoIPCache = make(map[string]string)
	geoIPCacheMu.Unlock()
}

// GetCachedRegion 获取缓存中的地区信息（不触发查询）
func GetCachedRegion(ip string) (string, bool) {
	geoIPCacheMu.RLock()
	defer geoIPCacheMu.RUnlock()
	region, ok := geoIPCache[ip]
	return region, ok
}

// PreloadIPRegion 预加载 IP 地区信息（异步，不阻塞）
func PreloadIPRegion(ip string) {
	go func() {
		_ = LookupIPRegion(ip)
	}()
}

// LookupIPRegionBatch 批量查询 IP 地区（并发）
func LookupIPRegionBatch(ips []string) map[string]string {
	result := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 限制并发数
	sem := make(chan struct{}, 5)

	for _, ip := range ips {
		if ip == "" {
			continue
		}

		wg.Add(1)
		go func(ipAddr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			region := LookupIPRegion(ipAddr)
			mu.Lock()
			result[ipAddr] = region
			mu.Unlock()
		}(ip)
	}

	wg.Wait()
	return result
}

// EnrichClientIPRegion 为单个 IP 地址获取地区信息
// 这是一个便捷函数，用于在返回客户端信息时填充 ip_region 字段
func EnrichClientIPRegion(ipAddress string) string {
	if ipAddress == "" {
		return ""
	}
	// 处理可能带端口的情况
	ip := ipAddress
	if strings.Contains(ipAddress, ":") && !strings.Contains(ipAddress, "[") {
		// IPv4:port 格式
		host, _, err := net.SplitHostPort(ipAddress)
		if err == nil {
			ip = host
		}
	}
	return LookupIPRegion(ip)
}

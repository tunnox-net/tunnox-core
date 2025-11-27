package storage

import "time"

// DataCategory 数据分类
type DataCategory int

const (
	// DataCategoryRuntime 运行时数据（仅缓存）
	DataCategoryRuntime DataCategory = iota
	// DataCategoryPersistent 持久化数据（数据库+缓存）
	DataCategoryPersistent
)

// HybridConfig 混合存储配置
type HybridConfig struct {
	// 持久化数据的 key 前缀列表
	PersistentPrefixes []string
	
	// 默认缓存 TTL
	DefaultCacheTTL time.Duration
	
	// 持久化数据的缓存 TTL
	PersistentCacheTTL time.Duration
	
	// 是否启用持久化存储（false 则为纯内存模式）
	EnablePersistent bool
}

// DefaultHybridConfig 返回默认配置
func DefaultHybridConfig() *HybridConfig {
	return &HybridConfig{
		PersistentPrefixes: []string{
			"tunnox:user:",               // 用户信息
			"tunnox:client:",             // 客户端配置
			"tunnox:mapping:",            // 端口映射配置
			"tunnox:node:",               // 节点信息
			"tunnox:stats:persistent:",   // 持久化统计数据
		},
		DefaultCacheTTL:    1 * time.Hour,
		PersistentCacheTTL: 24 * time.Hour,
		EnablePersistent:   false, // 默认纯内存模式
	}
}

// RuntimePrefixes 运行时数据的 key 前缀（仅用于文档说明）
var RuntimePrefixes = []string{
	"tunnox:runtime:",         // 运行时数据（加密密钥等）
	"tunnox:session:",         // 会话信息
	"tunnox:jwt:",             // JWT Token 缓存
	"tunnox:route:",           // 客户端路由信息
	"tunnox:temp:",            // 临时状态
	"tunnox:stats:runtime:",   // 运行时统计数据
	"tunnox:stats:cache:",     // 统计缓存
}


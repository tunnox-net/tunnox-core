package storage

import "time"

// DataCategory 数据分类
type DataCategory int

const (
	// DataCategoryRuntime 运行时数据（仅本地缓存）
	DataCategoryRuntime DataCategory = iota
	// DataCategoryPersistent 持久化数据（数据库+本地缓存）
	DataCategoryPersistent
	// DataCategoryShared 共享数据（仅共享缓存，用于纯运行时跨节点通信）
	DataCategoryShared
	// DataCategorySharedPersistent 共享且持久化数据（共享缓存+持久化存储）
	// 热点缓存模式：读取时先查共享缓存，miss则查持久化存储并回填共享缓存
	DataCategorySharedPersistent
)

// HybridConfig 混合存储配置
type HybridConfig struct {
	// 持久化数据的 key 前缀列表
	PersistentPrefixes []string

	// 共享数据的 key 前缀列表（跨节点共享，必须写入 Redis）
	// 这些数据不需要持久化，但需要在多节点间共享
	SharedPrefixes []string

	// 共享且持久化的数据前缀列表
	// 这些数据既需要跨节点共享（写入 Redis），也需要持久化（写入数据库）
	// 读取时采用热点缓存模式：共享缓存 -> 持久化存储 -> 回填共享缓存
	SharedPersistentPrefixes []string

	// 默认缓存 TTL
	DefaultCacheTTL time.Duration

	// 持久化数据的缓存 TTL
	PersistentCacheTTL time.Duration

	// 共享缓存的 TTL（用于热点缓存回填）
	SharedCacheTTL time.Duration

	// 是否启用持久化存储（false 则为纯内存模式）
	EnablePersistent bool
}

// DefaultHybridConfig 返回默认配置
func DefaultHybridConfig() *HybridConfig {
	return &HybridConfig{
		PersistentPrefixes: []string{
			"tunnox:user:",                  // 用户信息
			"tunnox:client:",                // 客户端配置（旧版，逐步废弃）
			"tunnox:config:client:",         // 客户端配置（新版）
			"tunnox:persist:client:config:", // 客户端持久化配置（实际使用的key）
			"tunnox:persist:clients:list",   // 客户端列表（实际使用的key）
			"tunnox:mapping:",               // 端口映射配置（旧key，仅持久化）
			"tunnox:persist:mapping:",       // 端口映射持久化配置
			"tunnox:persist:mappings:list",  // 端口映射列表（持久化备份）
			"tunnox:stats:persistent:",      // 持久化统计数据
		},
		SharedPrefixes: []string{
			"tunnox:conn_state:",            // 连接状态（跨节点查询，纯运行时）
			"tunnox:client_conn:",           // 客户端连接索引（跨节点查询，纯运行时）
			"tunnox:tunnel_waiting:",        // 隧道等待状态（跨节点路由，纯运行时）
			"tunnox:node:",                  // 节点信息（地址、ID分配锁，跨节点共享）
			"tunnox:runtime:conncode:",      // 连接码（跨节点共享，用于激活流程）
			"tunnox:index:conncode:target:", // 连接码索引（跨节点列表查询）
			"tunnox:id:",                    // ID生成器（集群模式全局唯一）
			"tunnox:runtime:client:state:",  // 客户端运行时状态（跨节点状态查询）
		},
		SharedPersistentPrefixes: []string{
			"tunnox:client_mappings:", // 客户端映射索引（跨节点共享 + 持久化）
			"tunnox:user_mappings:",   // 用户映射索引（跨节点共享 + 持久化）
			"tunnox:port_mapping:",    // 端口映射配置（跨节点访问 + 持久化）
			"tunnox:mappings:list",    // 映射全局列表（跨节点查询 + 持久化）
		},
		DefaultCacheTTL:    1 * time.Hour,
		PersistentCacheTTL: 24 * time.Hour,
		SharedCacheTTL:     1 * time.Hour, // 共享缓存热点 TTL
		EnablePersistent:   false,         // 默认纯内存模式
	}
}

// RuntimePrefixes 运行时数据的 key 前缀（仅用于文档说明）
var RuntimePrefixes = []string{
	"tunnox:runtime:",       // 运行时数据（加密密钥等）
	"tunnox:session:",       // 会话信息
	"tunnox:jwt:",           // JWT Token 缓存
	"tunnox:route:",         // 客户端路由信息
	"tunnox:temp:",          // 临时状态
	"tunnox:stats:runtime:", // 运行时统计数据
	"tunnox:stats:cache:",   // 统计缓存
}

package repos

import (
	"context"
	"fmt"
	"time"
)

// =============================================================================
// HTTP 域名映射数据模型和存储键定义
// =============================================================================
//
// HTTPDomainMapping 用于管理 HTTP 反向代理的域名映射关系。
// 每个客户端可以注册一个或多个子域名，将 HTTP 请求代理到内网服务。
//
// 存储架构：
// - 映射数据：SharedPersistent（持久化 + 缓存）
// - 域名索引：Shared（仅缓存，用于 O(1) 查找）
// - 客户端索引：SharedPersistent（持久化客户端的映射列表）
// - ID 生成器：Shared（自增 ID）
//
// =============================================================================

// =============================================================================
// 存储键常量
// =============================================================================

const (
	// KeyPrefixHTTPDomainMapping 映射数据存储键前缀（SharedPersistent）
	// 格式：tunnox:http_domain:mapping:{mapping_id}
	// 存储完整的 HTTPDomainMapping JSON
	KeyPrefixHTTPDomainMapping = "tunnox:http_domain:mapping:"

	// KeyPrefixHTTPDomainIndex 域名到映射ID的索引（Shared）
	// 格式：tunnox:http_domain:index:{full_domain}
	// 值：mapping_id
	// 用于 O(1) 时间复杂度查找域名对应的映射
	KeyPrefixHTTPDomainIndex = "tunnox:http_domain:index:"

	// KeyPrefixHTTPDomainClient 客户端的映射列表（SharedPersistent）
	// 格式：tunnox:http_domain:client:{client_id}
	// 值：映射ID列表
	// 用于查询某个客户端的所有域名映射
	KeyPrefixHTTPDomainClient = "tunnox:http_domain:client:"

	// KeyHTTPDomainNextID ID 生成器（Shared）
	// 格式：tunnox:http_domain:next_id
	// 值：下一个可用的映射 ID 数字
	// 用于生成唯一的映射 ID
	KeyHTTPDomainNextID = "tunnox:http_domain:next_id"

	// KeyHTTPDomainMappingList 所有映射列表（SharedPersistent）
	// 格式：tunnox:http_domain:mappings:list
	// 值：所有映射的列表
	KeyHTTPDomainMappingList = "tunnox:http_domain:mappings:list"
)

// =============================================================================
// 辅助函数：生成存储键
// =============================================================================

// HTTPDomainMappingKey 生成映射数据的存储键
// 格式：tunnox:http_domain:mapping:{mapping_id}
func HTTPDomainMappingKey(mappingID string) string {
	return KeyPrefixHTTPDomainMapping + mappingID
}

// HTTPDomainIndexKey 生成域名索引的存储键
// 格式：tunnox:http_domain:index:{full_domain}
func HTTPDomainIndexKey(fullDomain string) string {
	return KeyPrefixHTTPDomainIndex + fullDomain
}

// HTTPDomainClientKey 生成客户端映射列表的存储键
// 格式：tunnox:http_domain:client:{client_id}
func HTTPDomainClientKey(clientID int64) string {
	return fmt.Sprintf("%s%d", KeyPrefixHTTPDomainClient, clientID)
}

// =============================================================================
// 数据模型
// =============================================================================

// HTTPDomainMappingStatus 映射状态
type HTTPDomainMappingStatus string

const (
	// HTTPDomainMappingStatusActive 活跃状态，可正常代理
	HTTPDomainMappingStatusActive HTTPDomainMappingStatus = "active"

	// HTTPDomainMappingStatusInactive 非活跃状态，暂停代理
	HTTPDomainMappingStatusInactive HTTPDomainMappingStatus = "inactive"

	// HTTPDomainMappingStatusExpired 已过期
	HTTPDomainMappingStatusExpired HTTPDomainMappingStatus = "expired"
)

// HTTPDomainMapping HTTP 域名映射信息
//
// 用于存储 HTTP 反向代理的域名映射配置。
// 客户端可以通过此结构将子域名（如 abc123.tunnox.net）映射到内网服务。
//
// 示例：
//   - Subdomain: "myapp"
//   - BaseDomain: "tunnox.net"
//   - FullDomain: "myapp.tunnox.net"
//   - Target: "localhost:8080"
//
// 访问 http://myapp.tunnox.net 的请求将被代理到客户端内网的 localhost:8080
type HTTPDomainMapping struct {
	// ID 映射唯一标识符
	// 格式：hdm_{数字}，如 hdm_1、hdm_2
	ID string `json:"id"`

	// Subdomain 子域名部分
	// 例如：如果完整域名是 abc123.tunnox.net，则 Subdomain 为 "abc123"
	Subdomain string `json:"subdomain"`

	// BaseDomain 基础域名
	// 例如："tunnox.net"、"tunnel.example.com"
	BaseDomain string `json:"base_domain"`

	// FullDomain 完整域名（由 Subdomain + "." + BaseDomain 组成）
	// 例如："abc123.tunnox.net"
	// 此字段在创建时自动生成，用于快速查询
	FullDomain string `json:"full_domain"`

	// ClientID 拥有此映射的客户端 ID
	ClientID int64 `json:"client_id"`

	// TargetHost 目标主机地址
	// 例如："localhost"、"192.168.1.100"
	TargetHost string `json:"target_host"`

	// TargetPort 目标端口
	// 例如：8080、3000
	TargetPort int `json:"target_port"`

	// Description 映射描述（可选）
	Description string `json:"description,omitempty"`

	// Status 映射状态
	Status HTTPDomainMappingStatus `json:"status"`

	// CreatedAt 创建时间（Unix 时间戳，秒）
	CreatedAt int64 `json:"created_at"`

	// UpdatedAt 最后更新时间（Unix 时间戳，秒）
	UpdatedAt int64 `json:"updated_at"`

	// ExpiresAt 过期时间（Unix 时间戳，秒，0 表示永不过期）
	ExpiresAt int64 `json:"expires_at,omitempty"`
}

// =============================================================================
// HTTPDomainMapping 方法
// =============================================================================

// Target 返回目标地址字符串
// 格式：host:port
func (m *HTTPDomainMapping) Target() string {
	return fmt.Sprintf("%s:%d", m.TargetHost, m.TargetPort)
}

// TargetURL 返回目标 URL
// 格式：http://host:port
func (m *HTTPDomainMapping) TargetURL() string {
	return fmt.Sprintf("http://%s:%d", m.TargetHost, m.TargetPort)
}

// IsExpired 检查映射是否已过期
func (m *HTTPDomainMapping) IsExpired() bool {
	if m.ExpiresAt == 0 {
		return false // 永不过期
	}
	return time.Now().Unix() > m.ExpiresAt
}

// IsActive 检查映射是否处于活跃状态且未过期
func (m *HTTPDomainMapping) IsActive() bool {
	return m.Status == HTTPDomainMappingStatusActive && !m.IsExpired()
}

// TimeRemaining 返回距离过期的剩余时间
// 如果已过期或永不过期，返回 0
func (m *HTTPDomainMapping) TimeRemaining() time.Duration {
	if m.ExpiresAt == 0 {
		return 0 // 永不过期，返回0表示无限
	}
	remaining := m.ExpiresAt - time.Now().Unix()
	if remaining <= 0 {
		return 0
	}
	return time.Duration(remaining) * time.Second
}

// Validate 验证映射数据的完整性
func (m *HTTPDomainMapping) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("mapping ID is required")
	}
	if m.Subdomain == "" {
		return fmt.Errorf("subdomain is required")
	}
	if m.BaseDomain == "" {
		return fmt.Errorf("base domain is required")
	}
	if m.FullDomain == "" {
		return fmt.Errorf("full domain is required")
	}
	if m.ClientID <= 0 {
		return fmt.Errorf("client ID must be positive")
	}
	if m.TargetHost == "" {
		return fmt.Errorf("target host is required")
	}
	if m.TargetPort <= 0 || m.TargetPort > 65535 {
		return fmt.Errorf("target port must be between 1 and 65535")
	}
	return nil
}

// =============================================================================
// Repository 接口定义
// =============================================================================

// IHTTPDomainMappingRepository 定义 HTTP 域名映射的存储接口
//
// 职责：
//   - 管理 HTTPDomainMapping 的 CRUD 操作
//   - 维护域名索引，支持 O(1) 域名查找
//   - 维护客户端映射列表索引
//   - 提供子域名可用性检查
//   - 提供基础域名列表查询
type IHTTPDomainMappingRepository interface {
	// CheckSubdomainAvailable 检查子域名是否可用
	//
	// 参数：
	//   - ctx: 上下文
	//   - subdomain: 子域名，如 "myapp"
	//   - baseDomain: 基础域名，如 "tunnox.net"
	//
	// 返回：
	//   - bool: true 表示可用，false 表示已被占用
	//   - error: 存储错误
	CheckSubdomainAvailable(ctx context.Context, subdomain string, baseDomain string) (bool, error)

	// CreateMapping 创建域名映射
	//
	// 此操作是原子的，确保：
	//   1. 域名唯一性（不会创建重复域名的映射）
	//   2. 同时更新映射数据和所有索引
	//
	// 参数：
	//   - ctx: 上下文
	//   - clientID: 客户端 ID
	//   - subdomain: 子域名
	//   - baseDomain: 基础域名
	//   - targetHost: 目标主机
	//   - targetPort: 目标端口
	//
	// 返回：
	//   - *HTTPDomainMapping: 创建的映射
	//   - error: 域名冲突或存储错误
	CreateMapping(ctx context.Context, clientID int64, subdomain, baseDomain, targetHost string, targetPort int) (*HTTPDomainMapping, error)

	// GetMapping 获取映射详情
	//
	// 参数：
	//   - ctx: 上下文
	//   - mappingID: 映射 ID
	//
	// 返回：
	//   - *HTTPDomainMapping: 映射详情
	//   - error: 映射不存在或存储错误
	GetMapping(ctx context.Context, mappingID string) (*HTTPDomainMapping, error)

	// GetMappingsByClientID 获取客户端的所有映射
	//
	// 参数：
	//   - ctx: 上下文
	//   - clientID: 客户端 ID
	//
	// 返回：
	//   - []*HTTPDomainMapping: 映射列表（可能为空）
	//   - error: 存储错误
	GetMappingsByClientID(ctx context.Context, clientID int64) ([]*HTTPDomainMapping, error)

	// UpdateMapping 更新映射
	//
	// 只允许更新以下字段：
	//   - TargetHost
	//   - TargetPort
	//   - Description
	//   - Status
	//
	// 不允许更新：Subdomain、BaseDomain、FullDomain、ClientID
	//
	// 参数：
	//   - ctx: 上下文
	//   - mapping: 更新后的映射
	//
	// 返回：
	//   - error: 映射不存在或存储错误
	UpdateMapping(ctx context.Context, mapping *HTTPDomainMapping) error

	// DeleteMapping 删除映射
	//
	// 此操作会同时删除：
	//   - 映射数据
	//   - 域名索引
	//   - 客户端映射列表中的引用
	//
	// 参数：
	//   - ctx: 上下文
	//   - mappingID: 映射 ID
	//   - clientID: 客户端 ID（用于权限验证）
	//
	// 返回：
	//   - error: 映射不存在、权限不足或存储错误
	DeleteMapping(ctx context.Context, mappingID string, clientID int64) error

	// LookupByDomain 根据域名查找映射
	//
	// 使用域名索引进行 O(1) 查找。
	//
	// 参数：
	//   - ctx: 上下文
	//   - fullDomain: 完整域名，如 "myapp.tunnox.net"
	//
	// 返回：
	//   - *HTTPDomainMapping: 映射详情
	//   - error: 域名未找到或存储错误
	LookupByDomain(ctx context.Context, fullDomain string) (*HTTPDomainMapping, error)

	// GetBaseDomains 获取所有支持的基础域名
	//
	// 返回系统配置的基础域名列表。
	//
	// 返回：
	//   - []string: 基础域名列表，如 ["tunnox.net", "tunnel.example.com"]
	GetBaseDomains() []string

	// ListAllMappings 列出所有映射
	//
	// 参数：
	//   - ctx: 上下文
	//
	// 返回：
	//   - []*HTTPDomainMapping: 所有映射列表
	//   - error: 存储错误
	ListAllMappings(ctx context.Context) ([]*HTTPDomainMapping, error)

	// CleanupExpiredMappings 清理过期的映射
	//
	// 参数：
	//   - ctx: 上下文
	//
	// 返回：
	//   - int: 清理的映射数量
	//   - error: 存储错误
	CleanupExpiredMappings(ctx context.Context) (int, error)
}

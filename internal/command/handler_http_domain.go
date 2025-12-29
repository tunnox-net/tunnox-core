package command

import (
	"math/rand"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射命令处理器
// 本文件包含公共类型和接口定义
// 具体 Handler 实现分布在以下文件：
// - handler_http_domain_create.go: 创建相关 Handler
// - handler_http_domain_query.go: 查询相关 Handler
// - handler_http_domain_delete.go: 删除相关 Handler
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SubdomainChecker 子域名检查接口
type SubdomainChecker interface {
	IsSubdomainAvailable(subdomain, baseDomain string) bool
	IsBaseDomainAllowed(baseDomain string) bool
}

// HTTPDomainCreator HTTP 域名映射创建接口
type HTTPDomainCreator interface {
	CreateHTTPDomainMapping(clientID int64, targetHost string, targetPort int, subdomain, baseDomain, description string, ttlSeconds int) (mappingID, fullDomain, expiresAt string, err error)
}

// HTTPDomainLister HTTP 域名映射列表查询接口
type HTTPDomainLister interface {
	ListHTTPDomainMappings(clientID int64) ([]HTTPDomainMappingInfo, error)
}

// HTTPDomainMappingInfo HTTP 域名映射信息
type HTTPDomainMappingInfo struct {
	MappingID  string `json:"mapping_id"`
	FullDomain string `json:"full_domain"`
	TargetURL  string `json:"target_url"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

// HTTPDomainDeleter HTTP 域名映射删除接口
type HTTPDomainDeleter interface {
	DeleteHTTPDomainMapping(clientID int64, mappingID string) error
}

// generateRandomSubdomain 生成随机子域名
func generateRandomSubdomain() string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	const length = 4
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return "s" + string(result)
}

func init() {
	// 初始化随机数种子
	rand.Seed(time.Now().UnixNano())
}

package models

// IngressConfig 入口配置（根据 Protocol 使用不同字段）
type IngressConfig struct {
	// 端口类型入口 (tcp/udp/socks)
	ListenPort int `json:"listen_port,omitempty"`

	// 域名类型入口 (http)
	Subdomain  string `json:"subdomain,omitempty"`   // 如 "myapp"
	BaseDomain string `json:"base_domain,omitempty"` // 如 "tunnel.example.com"
}

// EgressConfig 出口配置（统一）
type EgressConfig struct {
	Host string `json:"host"` // 内网目标地址
	Port int    `json:"port"` // 内网目标端口
}

// Ingress 获取入口配置（兼容旧字段）
func (m *PortMapping) Ingress() IngressConfig {
	return IngressConfig{
		ListenPort: m.SourcePort,
		Subdomain:  m.HTTPSubdomain,
		BaseDomain: m.HTTPBaseDomain,
	}
}

// Egress 获取出口配置（兼容旧字段）
func (m *PortMapping) Egress() EgressConfig {
	return EgressConfig{
		Host: m.TargetHost,
		Port: m.TargetPort,
	}
}

// FullDomain 获取完整域名（仅 HTTP 协议有效）
func (m *PortMapping) FullDomain() string {
	if m.Protocol != ProtocolHTTP {
		return ""
	}
	if m.HTTPSubdomain == "" || m.HTTPBaseDomain == "" {
		return ""
	}
	return m.HTTPSubdomain + "." + m.HTTPBaseDomain
}

// SetIngress 设置入口配置
func (m *PortMapping) SetIngress(ingress IngressConfig) {
	m.SourcePort = ingress.ListenPort
	m.HTTPSubdomain = ingress.Subdomain
	m.HTTPBaseDomain = ingress.BaseDomain
}

// SetEgress 设置出口配置
func (m *PortMapping) SetEgress(egress EgressConfig) {
	m.TargetHost = egress.Host
	m.TargetPort = egress.Port
}

// IsHTTPMapping 检查是否为 HTTP 域名映射
func (m *PortMapping) IsHTTPMapping() bool {
	return m.Protocol == ProtocolHTTP
}

// IsPortMapping 检查是否为端口映射
func (m *PortMapping) IsPortMapping() bool {
	return m.Protocol == ProtocolTCP || m.Protocol == ProtocolSOCKS
}

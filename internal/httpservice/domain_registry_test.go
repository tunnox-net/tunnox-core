package httpservice

import (
	"testing"

	"tunnox-core/internal/cloud/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDomainRegistry_Register(t *testing.T) {
	registry := NewDomainRegistry([]string{"tunnel.example.com"})

	mapping := &models.PortMapping{
		ID:             "pm_123",
		Protocol:       models.ProtocolHTTP,
		HTTPSubdomain:  "myapp",
		HTTPBaseDomain: "tunnel.example.com",
		TargetClientID: 12345,
		TargetHost:     "localhost",
		TargetPort:     8080,
		Status:         models.MappingStatusActive,
	}

	// 注册成功
	err := registry.Register(mapping)
	require.NoError(t, err)

	// 查找成功
	found, ok := registry.Lookup("myapp.tunnel.example.com")
	assert.True(t, ok)
	assert.Equal(t, mapping.ID, found.ID)

	// 重复注册同一个映射（更新）
	err = registry.Register(mapping)
	require.NoError(t, err)

	// 注册不同映射到同一域名
	mapping2 := &models.PortMapping{
		ID:             "pm_456",
		Protocol:       models.ProtocolHTTP,
		HTTPSubdomain:  "myapp",
		HTTPBaseDomain: "tunnel.example.com",
		TargetClientID: 67890,
		TargetHost:     "localhost",
		TargetPort:     9090,
		Status:         models.MappingStatusActive,
	}
	err = registry.Register(mapping2)
	assert.Error(t, err)
}

func TestDomainRegistry_Unregister(t *testing.T) {
	registry := NewDomainRegistry([]string{"tunnel.example.com"})

	mapping := &models.PortMapping{
		ID:             "pm_123",
		Protocol:       models.ProtocolHTTP,
		HTTPSubdomain:  "myapp",
		HTTPBaseDomain: "tunnel.example.com",
		TargetClientID: 12345,
		TargetHost:     "localhost",
		TargetPort:     8080,
		Status:         models.MappingStatusActive,
	}

	// 注册
	err := registry.Register(mapping)
	require.NoError(t, err)

	// 注销
	registry.Unregister("myapp.tunnel.example.com")

	// 查找失败
	_, ok := registry.Lookup("myapp.tunnel.example.com")
	assert.False(t, ok)
}

func TestDomainRegistry_LookupByHost(t *testing.T) {
	registry := NewDomainRegistry([]string{"tunnel.example.com"})

	mapping := &models.PortMapping{
		ID:             "pm_123",
		Protocol:       models.ProtocolHTTP,
		HTTPSubdomain:  "myapp",
		HTTPBaseDomain: "tunnel.example.com",
		TargetClientID: 12345,
		TargetHost:     "localhost",
		TargetPort:     8080,
		Status:         models.MappingStatusActive,
	}

	err := registry.Register(mapping)
	require.NoError(t, err)

	// 不带端口
	found, ok := registry.LookupByHost("myapp.tunnel.example.com")
	assert.True(t, ok)
	assert.Equal(t, mapping.ID, found.ID)

	// 带端口
	found, ok = registry.LookupByHost("myapp.tunnel.example.com:443")
	assert.True(t, ok)
	assert.Equal(t, mapping.ID, found.ID)
}

func TestDomainRegistry_BaseDomainValidation(t *testing.T) {
	registry := NewDomainRegistry([]string{"tunnel.example.com"})

	// 允许的基础域名
	assert.True(t, registry.IsBaseDomainAllowed("tunnel.example.com"))

	// 不允许的基础域名
	assert.False(t, registry.IsBaseDomainAllowed("other.example.com"))

	// 注册不允许的基础域名
	mapping := &models.PortMapping{
		ID:             "pm_123",
		Protocol:       models.ProtocolHTTP,
		HTTPSubdomain:  "myapp",
		HTTPBaseDomain: "other.example.com",
		TargetClientID: 12345,
		TargetHost:     "localhost",
		TargetPort:     8080,
		Status:         models.MappingStatusActive,
	}

	err := registry.Register(mapping)
	assert.Error(t, err)
}

func TestDomainRegistry_Rebuild(t *testing.T) {
	registry := NewDomainRegistry([]string{"tunnel.example.com"})

	mappings := []*models.PortMapping{
		{
			ID:             "pm_1",
			Protocol:       models.ProtocolHTTP,
			HTTPSubdomain:  "app1",
			HTTPBaseDomain: "tunnel.example.com",
			TargetClientID: 1,
			TargetHost:     "localhost",
			TargetPort:     8081,
		},
		{
			ID:             "pm_2",
			Protocol:       models.ProtocolHTTP,
			HTTPSubdomain:  "app2",
			HTTPBaseDomain: "tunnel.example.com",
			TargetClientID: 2,
			TargetHost:     "localhost",
			TargetPort:     8082,
		},
		{
			ID:             "pm_3",
			Protocol:       models.ProtocolTCP, // 非 HTTP 协议，应该被忽略
			TargetClientID: 3,
			TargetHost:     "localhost",
			TargetPort:     8083,
		},
	}

	registry.Rebuild(mappings)

	assert.Equal(t, 2, registry.Count())

	_, ok := registry.Lookup("app1.tunnel.example.com")
	assert.True(t, ok)

	_, ok = registry.Lookup("app2.tunnel.example.com")
	assert.True(t, ok)
}

func TestDomainRegistry_IsSubdomainAvailable(t *testing.T) {
	registry := NewDomainRegistry([]string{"tunnel.example.com"})

	// 初始状态，子域名可用
	assert.True(t, registry.IsSubdomainAvailable("myapp", "tunnel.example.com"))

	// 注册后，子域名不可用
	mapping := &models.PortMapping{
		ID:             "pm_123",
		Protocol:       models.ProtocolHTTP,
		HTTPSubdomain:  "myapp",
		HTTPBaseDomain: "tunnel.example.com",
		TargetClientID: 12345,
		TargetHost:     "localhost",
		TargetPort:     8080,
		Status:         models.MappingStatusActive,
	}

	err := registry.Register(mapping)
	require.NoError(t, err)

	assert.False(t, registry.IsSubdomainAvailable("myapp", "tunnel.example.com"))
	assert.True(t, registry.IsSubdomainAvailable("otherapp", "tunnel.example.com"))
}

func TestDomainRegistry_GetMappingsByClientID(t *testing.T) {
	registry := NewDomainRegistry([]string{"tunnel.example.com"})

	mappings := []*models.PortMapping{
		{
			ID:             "pm_1",
			Protocol:       models.ProtocolHTTP,
			HTTPSubdomain:  "app1",
			HTTPBaseDomain: "tunnel.example.com",
			TargetClientID: 100,
			TargetHost:     "localhost",
			TargetPort:     8081,
			Status:         models.MappingStatusActive,
		},
		{
			ID:             "pm_2",
			Protocol:       models.ProtocolHTTP,
			HTTPSubdomain:  "app2",
			HTTPBaseDomain: "tunnel.example.com",
			TargetClientID: 100,
			TargetHost:     "localhost",
			TargetPort:     8082,
			Status:         models.MappingStatusActive,
		},
		{
			ID:             "pm_3",
			Protocol:       models.ProtocolHTTP,
			HTTPSubdomain:  "app3",
			HTTPBaseDomain: "tunnel.example.com",
			TargetClientID: 200,
			TargetHost:     "localhost",
			TargetPort:     8083,
			Status:         models.MappingStatusActive,
		},
	}

	for _, m := range mappings {
		err := registry.Register(m)
		require.NoError(t, err)
	}

	// 获取客户端 100 的映射
	client100Mappings := registry.GetMappingsByClientID(100)
	assert.Len(t, client100Mappings, 2)

	// 获取客户端 200 的映射
	client200Mappings := registry.GetMappingsByClientID(200)
	assert.Len(t, client200Mappings, 1)

	// 获取不存在的客户端
	client300Mappings := registry.GetMappingsByClientID(300)
	assert.Len(t, client300Mappings, 0)
}

func TestPortMapping_FullDomain(t *testing.T) {
	// HTTP 协议
	httpMapping := &models.PortMapping{
		Protocol:       models.ProtocolHTTP,
		HTTPSubdomain:  "myapp",
		HTTPBaseDomain: "tunnel.example.com",
	}
	assert.Equal(t, "myapp.tunnel.example.com", httpMapping.FullDomain())

	// TCP 协议
	tcpMapping := &models.PortMapping{
		Protocol:       models.ProtocolTCP,
		HTTPSubdomain:  "myapp",
		HTTPBaseDomain: "tunnel.example.com",
	}
	assert.Equal(t, "", tcpMapping.FullDomain())

	// 缺少子域名
	noSubdomain := &models.PortMapping{
		Protocol:       models.ProtocolHTTP,
		HTTPBaseDomain: "tunnel.example.com",
	}
	assert.Equal(t, "", noSubdomain.FullDomain())
}

func TestPortMapping_IngressEgress(t *testing.T) {
	mapping := &models.PortMapping{
		Protocol:       models.ProtocolHTTP,
		SourcePort:     8080,
		HTTPSubdomain:  "myapp",
		HTTPBaseDomain: "tunnel.example.com",
		TargetHost:     "localhost",
		TargetPort:     9090,
	}

	ingress := mapping.Ingress()
	assert.Equal(t, 8080, ingress.ListenPort)
	assert.Equal(t, "myapp", ingress.Subdomain)
	assert.Equal(t, "tunnel.example.com", ingress.BaseDomain)

	egress := mapping.Egress()
	assert.Equal(t, "localhost", egress.Host)
	assert.Equal(t, 9090, egress.Port)
}

func TestPortMapping_SetIngressEgress(t *testing.T) {
	mapping := &models.PortMapping{}

	mapping.SetIngress(models.IngressConfig{
		ListenPort: 8080,
		Subdomain:  "myapp",
		BaseDomain: "tunnel.example.com",
	})

	mapping.SetEgress(models.EgressConfig{
		Host: "localhost",
		Port: 9090,
	})

	assert.Equal(t, 8080, mapping.SourcePort)
	assert.Equal(t, "myapp", mapping.HTTPSubdomain)
	assert.Equal(t, "tunnel.example.com", mapping.HTTPBaseDomain)
	assert.Equal(t, "localhost", mapping.TargetHost)
	assert.Equal(t, 9090, mapping.TargetPort)
}

func TestPortMapping_IsHTTPMapping(t *testing.T) {
	httpMapping := &models.PortMapping{Protocol: models.ProtocolHTTP}
	assert.True(t, httpMapping.IsHTTPMapping())
	assert.False(t, httpMapping.IsPortMapping())

	tcpMapping := &models.PortMapping{Protocol: models.ProtocolTCP}
	assert.False(t, tcpMapping.IsHTTPMapping())
	assert.True(t, tcpMapping.IsPortMapping())

	socksMapping := &models.PortMapping{Protocol: models.ProtocolSOCKS}
	assert.False(t, socksMapping.IsHTTPMapping())
	assert.True(t, socksMapping.IsPortMapping())
}

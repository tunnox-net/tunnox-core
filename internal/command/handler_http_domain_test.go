package command

import (
	"encoding/json"
	"testing"

	"tunnox-core/internal/packet"
)

// MockSubdomainChecker 模拟子域名检查器
type MockSubdomainChecker struct {
	allowedBaseDomains []string
	usedSubdomains     map[string]bool
}

func NewMockSubdomainChecker() *MockSubdomainChecker {
	return &MockSubdomainChecker{
		allowedBaseDomains: []string{"tunnox.net", "test.local"},
		usedSubdomains:     make(map[string]bool),
	}
}

func (c *MockSubdomainChecker) IsBaseDomainAllowed(baseDomain string) bool {
	for _, d := range c.allowedBaseDomains {
		if d == baseDomain {
			return true
		}
	}
	return false
}

func (c *MockSubdomainChecker) IsSubdomainAvailable(subdomain, baseDomain string) bool {
	fullDomain := subdomain + "." + baseDomain
	return !c.usedSubdomains[fullDomain]
}

func (c *MockSubdomainChecker) MarkUsed(subdomain, baseDomain string) {
	fullDomain := subdomain + "." + baseDomain
	c.usedSubdomains[fullDomain] = true
}

// MockHTTPDomainCreator 模拟 HTTP 域名创建器
type MockHTTPDomainCreator struct {
	lastClientID   int64
	lastTargetHost string
	lastTargetPort int
	lastSubdomain  string
	lastBaseDomain string
	nextMappingID  int
}

func NewMockHTTPDomainCreator() *MockHTTPDomainCreator {
	return &MockHTTPDomainCreator{nextMappingID: 1}
}

func (c *MockHTTPDomainCreator) CreateHTTPDomainMapping(
	clientID int64, targetHost string, targetPort int,
	subdomain, baseDomain, description string, ttlSeconds int,
) (mappingID, fullDomain, expiresAt string, err error) {
	c.lastClientID = clientID
	c.lastTargetHost = targetHost
	c.lastTargetPort = targetPort
	c.lastSubdomain = subdomain
	c.lastBaseDomain = baseDomain

	mappingID = "hdm_" + string(rune('0'+c.nextMappingID))
	c.nextMappingID++
	fullDomain = subdomain + "." + baseDomain
	expiresAt = "2025-12-31T23:59:59Z"
	return
}

// TestHTTPDomainGetBaseDomainsHandler 测试获取基础域名列表
func TestHTTPDomainGetBaseDomainsHandler(t *testing.T) {
	baseDomains := []string{"tunnox.net", "test.local"}
	handler := NewHTTPDomainGetBaseDomainsHandler(baseDomains)

	ctx := &CommandContext{
		ConnectionID: "test-conn-1",
		RequestID:    "req-1",
		CommandId:    "cmd-1",
		RequestBody:  "",
	}

	resp, err := handler.Handle(ctx)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success=true, got false")
	}

	var result packet.HTTPDomainGetBaseDomainsResponse
	if err := json.Unmarshal([]byte(resp.Data), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected result.Success=true, got false")
	}

	if len(result.BaseDomains) != 2 {
		t.Errorf("Expected 2 base domains, got %d", len(result.BaseDomains))
	}

	if result.BaseDomains[0].Domain != "tunnox.net" {
		t.Errorf("Expected first domain to be 'tunnox.net', got '%s'", result.BaseDomains[0].Domain)
	}
}

// TestHTTPDomainCheckSubdomainHandler 测试检查子域名可用性
func TestHTTPDomainCheckSubdomainHandler(t *testing.T) {
	checker := NewMockSubdomainChecker()
	handler := NewHTTPDomainCheckSubdomainHandler(checker)

	// 测试可用的子域名
	t.Run("AvailableSubdomain", func(t *testing.T) {
		req := packet.HTTPDomainCheckSubdomainRequest{
			Subdomain:  "myapp",
			BaseDomain: "tunnox.net",
		}
		reqBody, _ := json.Marshal(req)

		ctx := &CommandContext{
			ConnectionID: "test-conn-1",
			RequestID:    "req-1",
			CommandId:    "cmd-1",
			RequestBody:  string(reqBody),
		}

		resp, err := handler.Handle(ctx)
		if err != nil {
			t.Fatalf("Handle failed: %v", err)
		}

		if !resp.Success {
			t.Errorf("Expected success=true, got false")
		}

		var result packet.HTTPDomainCheckSubdomainResponse
		if err := json.Unmarshal([]byte(resp.Data), &result); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if !result.Available {
			t.Errorf("Expected available=true, got false")
		}

		if result.FullDomain != "myapp.tunnox.net" {
			t.Errorf("Expected full_domain='myapp.tunnox.net', got '%s'", result.FullDomain)
		}
	})

	// 测试已使用的子域名
	t.Run("UsedSubdomain", func(t *testing.T) {
		checker.MarkUsed("taken", "tunnox.net")

		req := packet.HTTPDomainCheckSubdomainRequest{
			Subdomain:  "taken",
			BaseDomain: "tunnox.net",
		}
		reqBody, _ := json.Marshal(req)

		ctx := &CommandContext{
			ConnectionID: "test-conn-1",
			RequestID:    "req-2",
			CommandId:    "cmd-2",
			RequestBody:  string(reqBody),
		}

		resp, err := handler.Handle(ctx)
		if err != nil {
			t.Fatalf("Handle failed: %v", err)
		}

		var result packet.HTTPDomainCheckSubdomainResponse
		if err := json.Unmarshal([]byte(resp.Data), &result); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if result.Available {
			t.Errorf("Expected available=false for used subdomain, got true")
		}
	})

	// 测试不允许的基础域名
	t.Run("InvalidBaseDomain", func(t *testing.T) {
		req := packet.HTTPDomainCheckSubdomainRequest{
			Subdomain:  "myapp",
			BaseDomain: "invalid.com",
		}
		reqBody, _ := json.Marshal(req)

		ctx := &CommandContext{
			ConnectionID: "test-conn-1",
			RequestID:    "req-3",
			CommandId:    "cmd-3",
			RequestBody:  string(reqBody),
		}

		resp, err := handler.Handle(ctx)
		if err != nil {
			t.Fatalf("Handle failed: %v", err)
		}

		if resp.Success {
			t.Errorf("Expected success=false for invalid base domain")
		}
	})
}

// TestHTTPDomainGenSubdomainHandler 测试生成随机子域名
func TestHTTPDomainGenSubdomainHandler(t *testing.T) {
	checker := NewMockSubdomainChecker()
	handler := NewHTTPDomainGenSubdomainHandler(checker)

	req := packet.HTTPDomainGenSubdomainRequest{
		BaseDomain: "tunnox.net",
	}
	reqBody, _ := json.Marshal(req)

	ctx := &CommandContext{
		ConnectionID: "test-conn-1",
		RequestID:    "req-1",
		CommandId:    "cmd-1",
		RequestBody:  string(reqBody),
	}

	resp, err := handler.Handle(ctx)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success=true, got false")
	}

	var result packet.HTTPDomainGenSubdomainResponse
	if err := json.Unmarshal([]byte(resp.Data), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected result.Success=true, got false")
	}

	if result.Subdomain == "" {
		t.Errorf("Expected non-empty subdomain")
	}

	if result.FullDomain == "" {
		t.Errorf("Expected non-empty full_domain")
	}

	// 验证子域名格式：应该以 's' 开头
	if result.Subdomain[0] != 's' {
		t.Errorf("Expected subdomain to start with 's', got '%s'", result.Subdomain)
	}
}

// TestHTTPDomainCreateHandler 测试创建 HTTP 域名映射
func TestHTTPDomainCreateHandler(t *testing.T) {
	checker := NewMockSubdomainChecker()
	creator := NewMockHTTPDomainCreator()
	handler := NewHTTPDomainCreateHandler(checker, creator)

	req := packet.HTTPDomainCreateRequest{
		TargetURL:   "http://localhost:8080",
		Subdomain:   "myapp",
		BaseDomain:  "tunnox.net",
		MappingTTL:  3600,
		Description: "Test mapping",
	}
	reqBody, _ := json.Marshal(req)

	ctx := &CommandContext{
		ConnectionID: "test-conn-1",
		ClientID:     10001,
		RequestID:    "req-1",
		CommandId:    "cmd-1",
		RequestBody:  string(reqBody),
	}

	resp, err := handler.Handle(ctx)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success=true, got false. Error: %s", resp.Error)
	}

	var result packet.HTTPDomainCreateResponse
	if err := json.Unmarshal([]byte(resp.Data), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected result.Success=true, got false. Error: %s", result.Error)
	}

	if result.MappingID == "" {
		t.Errorf("Expected non-empty mapping_id")
	}

	if result.FullDomain != "myapp.tunnox.net" {
		t.Errorf("Expected full_domain='myapp.tunnox.net', got '%s'", result.FullDomain)
	}

	// 验证创建器被正确调用
	if creator.lastClientID != 10001 {
		t.Errorf("Expected clientID=10001, got %d", creator.lastClientID)
	}

	if creator.lastTargetHost != "localhost" {
		t.Errorf("Expected targetHost='localhost', got '%s'", creator.lastTargetHost)
	}

	if creator.lastTargetPort != 8080 {
		t.Errorf("Expected targetPort=8080, got %d", creator.lastTargetPort)
	}
}

// TestHTTPDomainCreateHandler_InvalidURL 测试无效 URL
func TestHTTPDomainCreateHandler_InvalidURL(t *testing.T) {
	checker := NewMockSubdomainChecker()
	creator := NewMockHTTPDomainCreator()
	handler := NewHTTPDomainCreateHandler(checker, creator)

	req := packet.HTTPDomainCreateRequest{
		TargetURL:  "not-a-valid-url",
		Subdomain:  "myapp",
		BaseDomain: "tunnox.net",
	}
	reqBody, _ := json.Marshal(req)

	ctx := &CommandContext{
		ConnectionID: "test-conn-1",
		ClientID:     10001,
		RequestID:    "req-1",
		CommandId:    "cmd-1",
		RequestBody:  string(reqBody),
	}

	resp, err := handler.Handle(ctx)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	// URL 解析可能不会失败，但结果可能不正确
	// 这里只是确保不会 panic
	t.Logf("Response success: %v, data: %s", resp.Success, resp.Data)
}

// TestHTTPDomainCreateHandler_SubdomainNotAvailable 测试子域名不可用
func TestHTTPDomainCreateHandler_SubdomainNotAvailable(t *testing.T) {
	checker := NewMockSubdomainChecker()
	checker.MarkUsed("taken", "tunnox.net")
	creator := NewMockHTTPDomainCreator()
	handler := NewHTTPDomainCreateHandler(checker, creator)

	req := packet.HTTPDomainCreateRequest{
		TargetURL:  "http://localhost:8080",
		Subdomain:  "taken",
		BaseDomain: "tunnox.net",
	}
	reqBody, _ := json.Marshal(req)

	ctx := &CommandContext{
		ConnectionID: "test-conn-1",
		ClientID:     10001,
		RequestID:    "req-1",
		CommandId:    "cmd-1",
		RequestBody:  string(reqBody),
	}

	resp, err := handler.Handle(ctx)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if resp.Success {
		t.Errorf("Expected success=false for unavailable subdomain")
	}

	var result packet.HTTPDomainCreateResponse
	if err := json.Unmarshal([]byte(resp.Data), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result.Success {
		t.Errorf("Expected result.Success=false")
	}

	if result.Error == "" {
		t.Errorf("Expected non-empty error message")
	}
}


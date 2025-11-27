package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// E2EAPIClient E2E测试API客户端
type E2EAPIClient struct {
	t       *testing.T
	baseURL string
	client  *http.Client
}

// NewE2EAPIClient 创建E2E API客户端
func NewE2EAPIClient(t *testing.T, baseURL string) *E2EAPIClient {
	return &E2EAPIClient{
		t:       t,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetAuth 设置认证token
func (c *E2EAPIClient) SetAuth(token string) {
	c.client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &authTransport{
			token: token,
			base:  http.DefaultTransport,
		},
	}
}

// authTransport 带认证的Transport
type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

// HealthCheck 健康检查
func (c *E2EAPIClient) HealthCheck() error {
	resp, err := c.client.Get(c.baseURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check failed: %d - %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// CreateUser 创建用户（强类型）
func (c *E2EAPIClient) CreateUser(req CreateUserRequest) (*UserResponse, error) {
	url := c.baseURL + "/api/v1/users"
	c.t.Logf("🌐 API Client: POST %s", url)
	
	body, _ := json.Marshal(req)
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		c.t.Logf("❌ API Client: POST failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()
	c.t.Logf("✅ API Client: POST response status=%d", resp.StatusCode)
	
	var apiResp struct {
		Success bool          `json:"success"`
		Data    *UserResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	
	if !apiResp.Success || apiResp.Data == nil {
		return nil, fmt.Errorf("create user failed")
	}
	
	return apiResp.Data, nil
}

// Login 用户登录（强类型）
func (c *E2EAPIClient) Login(username, password string) (string, error) {
	req := LoginRequest{
		Username: username,
		Password: password,
	}
	
	body, _ := json.Marshal(req)
	resp, err := c.client.Post(c.baseURL+"/api/v1/auth/login", 
		"application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	var apiResp struct {
		Success bool           `json:"success"`
		Data    *LoginResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", err
	}
	
	if !apiResp.Success || apiResp.Data == nil {
		return "", fmt.Errorf("login failed")
	}
	
	return apiResp.Data.Token, nil
}

// CreateClient 创建客户端（强类型）
func (c *E2EAPIClient) CreateClient(req CreateClientRequest) (*ClientResponse, error) {
	body, _ := json.Marshal(req)
	resp, err := c.client.Post(c.baseURL+"/api/v1/clients", 
		"application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var apiResp struct {
		Success bool            `json:"success"`
		Data    *ClientResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	
	if !apiResp.Success || apiResp.Data == nil {
		return nil, fmt.Errorf("create client failed")
	}
	
	return apiResp.Data, nil
}

// CreateMapping 创建端口映射（强类型）
func (c *E2EAPIClient) CreateMapping(req CreateMappingRequest) (*MappingResponse, error) {
	url := c.baseURL + "/api/v1/mappings"
	c.t.Logf("🌐 API Client: POST %s", url)
	c.t.Logf("🌐 API Client: Request body: source=%d, target=%d, port=%d", 
		req.SourceClientID, req.TargetClientID, req.SourcePort)
	
	body, _ := json.Marshal(req)
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		c.t.Logf("❌ API Client: POST failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()
	c.t.Logf("✅ API Client: POST response status=%d", resp.StatusCode)
	
	var apiResp struct {
		Success bool             `json:"success"`
		Data    *MappingResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	
	if !apiResp.Success || apiResp.Data == nil {
		return nil, fmt.Errorf("create mapping failed")
	}
	
	return apiResp.Data, nil
}

// ListClients 列出所有客户端（强类型）
func (c *E2EAPIClient) ListClients() ([]ClientResponse, error) {
	url := c.baseURL + "/api/v1/clients"
	c.t.Logf("🌐 API Client: GET %s", url)
	
	resp, err := c.client.Get(url)
	if err != nil {
		c.t.Logf("❌ API Client: GET failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()
	c.t.Logf("✅ API Client: GET response status=%d", resp.StatusCode)
	
	var apiResp struct {
		Success bool             `json:"success"`
		Data    struct {
			Clients []ClientResponse `json:"clients"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	
	if !apiResp.Success {
		return nil, fmt.Errorf("list clients failed")
	}
	
	return apiResp.Data.Clients, nil
}

// DeleteMapping 删除端口映射
func (c *E2EAPIClient) DeleteMapping(mappingID string) error {
	req, err := http.NewRequest("DELETE", c.baseURL+"/api/v1/mappings/"+mappingID, nil)
	if err != nil {
		return err
	}
	
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	var apiResp struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return err
	}
	
	if !apiResp.Success {
		return fmt.Errorf("delete mapping failed")
	}
	
	return nil
}

// ClaimClient 关联匿名客户端到用户（强类型）
func (c *E2EAPIClient) ClaimClient(clientID int64, userID string, newName string) (*ClaimClientResponse, error) {
	reqBody := map[string]interface{}{
		"user_id":  userID,
		"new_name": newName,
	}
	body, _ := json.Marshal(reqBody)
	
	url := fmt.Sprintf("%s/api/v1/clients/%d/claim", c.baseURL, clientID)
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var apiResp struct {
		Success bool                  `json:"success"`
		Data    *ClaimClientResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	
	if !apiResp.Success || apiResp.Data == nil {
		return nil, fmt.Errorf("claim client failed")
	}
	
	return apiResp.Data, nil
}

// Request 发送HTTP请求（通用方法）
func (c *E2EAPIClient) Request(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	
	return c.client.Do(req)
}


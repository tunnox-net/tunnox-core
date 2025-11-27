package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
)

// APIClient 测试用API客户端
type APIClient struct {
	*dispose.ResourceBase

	httpClient *http.Client
	baseURL    string
	authToken  string
}

// APIResponse 统一API响应结构
type APIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
}

// NewAPIClient 创建API客户端
func NewAPIClient(ctx context.Context, baseURL string) *APIClient {
	client := &APIClient{
		ResourceBase: dispose.NewResourceBase("APIClient"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}

	// 添加清理处理器
	client.AddCleanHandler(func() error {
		if client.httpClient != nil {
			client.httpClient.CloseIdleConnections()
		}
		return nil
	})

	client.Initialize(ctx)
	return client
}

// SetAuthToken 设置认证令牌
func (c *APIClient) SetAuthToken(token string) {
	c.authToken = token
}

// request 发送HTTP请求
func (c *APIClient) request(method, path string, body interface{}) (*APIResponse, *http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, fmt.Errorf("failed to read response body: %w", err)
	}

	// 处理空响应（如 204 No Content）
	if len(respBody) == 0 {
		return &APIResponse{
			Success: resp.StatusCode >= 200 && resp.StatusCode < 300,
		}, resp, nil
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, resp, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &apiResp, resp, nil
}

// ==================== 用户管理接口 ====================

// CreateUser 创建用户
func (c *APIClient) CreateUser(username, email string) (*models.User, error) {
	reqBody := map[string]string{
		"username": username,
		"email":    email,
	}

	apiResp, _, err := c.request("POST", "/users", reqBody)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var user models.User
	if err := json.Unmarshal(apiResp.Data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// GetUser 获取用户信息
func (c *APIClient) GetUser(userID string) (*models.User, error) {
	apiResp, _, err := c.request("GET", fmt.Sprintf("/users/%s", userID), nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var user models.User
	if err := json.Unmarshal(apiResp.Data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// UpdateUser 更新用户信息
func (c *APIClient) UpdateUser(userID string, updates map[string]interface{}) (*models.User, error) {
	apiResp, _, err := c.request("PUT", fmt.Sprintf("/users/%s", userID), updates)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var user models.User
	if err := json.Unmarshal(apiResp.Data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// DeleteUser 删除用户
func (c *APIClient) DeleteUser(userID string) error {
	apiResp, resp, err := c.request("DELETE", fmt.Sprintf("/users/%s", userID), nil)
	if err != nil {
		return err
	}

	// DELETE可能返回204 No Content
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if !apiResp.Success {
		return fmt.Errorf("API error: %s", apiResp.Error)
	}

	return nil
}

// ListUsers 列出用户
func (c *APIClient) ListUsers() ([]*models.User, error) {
	apiResp, _, err := c.request("GET", "/users", nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var result struct {
		Users []*models.User `json:"users"`
		Total int            `json:"total"`
	}
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal users: %w", err)
	}

	return result.Users, nil
}

// ==================== 客户端管理接口 ====================

// CreateClient 创建客户端
func (c *APIClient) CreateClient(userID, clientName string) (*models.Client, error) {
	reqBody := map[string]string{
		"user_id":     userID,
		"client_name": clientName,
	}

	apiResp, _, err := c.request("POST", "/clients", reqBody)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var client models.Client
	if err := json.Unmarshal(apiResp.Data, &client); err != nil {
		return nil, fmt.Errorf("failed to unmarshal client: %w", err)
	}

	return &client, nil
}

// GetClient 获取客户端信息
func (c *APIClient) GetClient(clientID int64) (*models.Client, error) {
	apiResp, _, err := c.request("GET", fmt.Sprintf("/clients/%d", clientID), nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var client models.Client
	if err := json.Unmarshal(apiResp.Data, &client); err != nil {
		return nil, fmt.Errorf("failed to unmarshal client: %w", err)
	}

	return &client, nil
}

// UpdateClient 更新客户端信息
func (c *APIClient) UpdateClient(clientID int64, updates map[string]interface{}) (*models.Client, error) {
	apiResp, _, err := c.request("PUT", fmt.Sprintf("/clients/%d", clientID), updates)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var client models.Client
	if err := json.Unmarshal(apiResp.Data, &client); err != nil {
		return nil, fmt.Errorf("failed to unmarshal client: %w", err)
	}

	return &client, nil
}

// DeleteClient 删除客户端
func (c *APIClient) DeleteClient(clientID int64) error {
	apiResp, resp, err := c.request("DELETE", fmt.Sprintf("/clients/%d", clientID), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if !apiResp.Success {
		return fmt.Errorf("API error: %s", apiResp.Error)
	}

	return nil
}

// ListClients 列出所有客户端
func (c *APIClient) ListClients() ([]*models.Client, error) {
	apiResp, _, err := c.request("GET", "/clients", nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var result struct {
		Clients []*models.Client `json:"clients"`
		Total   int              `json:"total"`
	}
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal clients: %w", err)
	}

	return result.Clients, nil
}

// ==================== 映射管理接口 ====================

// CreateMapping 创建端口映射
func (c *APIClient) CreateMapping(mapping *models.PortMapping) (*models.PortMapping, error) {
	apiResp, _, err := c.request("POST", "/mappings", mapping)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var result models.PortMapping
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mapping: %w", err)
	}

	return &result, nil
}

// GetMapping 获取端口映射信息
func (c *APIClient) GetMapping(mappingID string) (*models.PortMapping, error) {
	apiResp, _, err := c.request("GET", fmt.Sprintf("/mappings/%s", mappingID), nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var mapping models.PortMapping
	if err := json.Unmarshal(apiResp.Data, &mapping); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mapping: %w", err)
	}

	return &mapping, nil
}

// UpdateMapping 更新端口映射
func (c *APIClient) UpdateMapping(mappingID string, updates map[string]interface{}) (*models.PortMapping, error) {
	apiResp, _, err := c.request("PUT", fmt.Sprintf("/mappings/%s", mappingID), updates)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var mapping models.PortMapping
	if err := json.Unmarshal(apiResp.Data, &mapping); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mapping: %w", err)
	}

	return &mapping, nil
}

// DeleteMapping 删除端口映射
func (c *APIClient) DeleteMapping(mappingID string) error {
	apiResp, resp, err := c.request("DELETE", fmt.Sprintf("/mappings/%s", mappingID), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if !apiResp.Success {
		return fmt.Errorf("API error: %s", apiResp.Error)
	}

	return nil
}

// ListMappings 列出所有映射
func (c *APIClient) ListMappings() ([]*models.PortMapping, error) {
	apiResp, _, err := c.request("GET", "/mappings", nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var result struct {
		Mappings []*models.PortMapping `json:"mappings"`
		Total    int                   `json:"total"`
	}
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mappings: %w", err)
	}

	return result.Mappings, nil
}

// ==================== 统计接口 ====================

// GetUserStats 获取用户统计
func (c *APIClient) GetUserStats(userID string) (*stats.UserStats, error) {
	apiResp, _, err := c.request("GET", fmt.Sprintf("/stats/users/%s", userID), nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var userStats stats.UserStats
	if err := json.Unmarshal(apiResp.Data, &userStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user stats: %w", err)
	}

	return &userStats, nil
}

// GetClientStats 获取客户端统计
func (c *APIClient) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	apiResp, _, err := c.request("GET", fmt.Sprintf("/stats/clients/%d", clientID), nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var clientStats stats.ClientStats
	if err := json.Unmarshal(apiResp.Data, &clientStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal client stats: %w", err)
	}

	return &clientStats, nil
}

// GetSystemStats 获取系统统计
func (c *APIClient) GetSystemStats() (*stats.SystemStats, error) {
	apiResp, _, err := c.request("GET", "/stats/system", nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var systemStats stats.SystemStats
	if err := json.Unmarshal(apiResp.Data, &systemStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal system stats: %w", err)
	}

	return &systemStats, nil
}

// ==================== 搜索接口 ====================

// SearchUsers 搜索用户
func (c *APIClient) SearchUsers(keyword string) ([]*models.User, error) {
	apiResp, _, err := c.request("GET", fmt.Sprintf("/search/users?q=%s", keyword), nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var result struct {
		Users []*models.User `json:"users"`
		Total int            `json:"total"`
	}
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal users: %w", err)
	}

	return result.Users, nil
}

// SearchClients 搜索客户端
func (c *APIClient) SearchClients(keyword string) ([]*models.Client, error) {
	apiResp, _, err := c.request("GET", fmt.Sprintf("/search/clients?q=%s", keyword), nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var result struct {
		Clients []*models.Client `json:"clients"`
		Total   int              `json:"total"`
	}
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal clients: %w", err)
	}

	return result.Clients, nil
}

// SearchMappings 搜索映射
func (c *APIClient) SearchMappings(keyword string) ([]*models.PortMapping, error) {
	apiResp, _, err := c.request("GET", fmt.Sprintf("/search/mappings?q=%s", keyword), nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Error)
	}

	var result struct {
		Mappings []*models.PortMapping `json:"mappings"`
		Total    int                   `json:"total"`
	}
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mappings: %w", err)
	}

	return result.Mappings, nil
}

// ==================== 健康检查 ====================

// HealthCheck 健康检查
func (c *APIClient) HealthCheck() (bool, error) {
	apiResp, resp, err := c.request("GET", "/health", nil)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	return apiResp.Success, nil
}


package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Management API 客户端
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ManagementAPIClient Management API 客户端
type ManagementAPIClient struct {
	baseURL    string
	httpClient *http.Client
	clientID   int64
	authToken  string
}

// NewManagementAPIClient 创建Management API客户端
func NewManagementAPIClient(serverAddr string, clientID int64, authToken string) *ManagementAPIClient {
	// 构造Management API的baseURL
	// 假设Management API在8080端口
	baseURL := fmt.Sprintf("http://%s", serverAddr)

	return &ManagementAPIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		clientID:  clientID,
		authToken: authToken,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码相关API
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateCodeRequest 生成连接码请求
type GenerateCodeRequest struct {
	TargetAddress string `json:"target_address"` // 目标地址（如 tcp://192.168.1.10:8080）
	ActivationTTL int    `json:"activation_ttl"` // 激活有效期（秒）
	MappingTTL    int    `json:"mapping_ttl"`    // 映射有效期（秒）
}

// GenerateCodeResponse 生成连接码响应
type GenerateCodeResponse struct {
	Code          string `json:"code"`
	TargetAddress string `json:"target_address"`
	ExpiresAt     string `json:"expires_at"`
}

// GenerateConnectionCode 生成连接码
func (c *ManagementAPIClient) GenerateConnectionCode(req *GenerateCodeRequest) (*GenerateCodeResponse, error) {
	url := fmt.Sprintf("%s/tunnox/v1/connection-codes", c.baseURL)

	respBody, err := c.doRequest("POST", url, req)
	if err != nil {
		return nil, err
	}

	var resp GenerateCodeResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// ListConnectionCodesResponse 连接码列表响应
type ListConnectionCodesResponse struct {
	Codes []ConnectionCodeInfo `json:"codes"`
	Total int                  `json:"total"`
}

// ConnectionCodeInfo 连接码信息
type ConnectionCodeInfo struct {
	Code          string `json:"code"`
	TargetAddress string `json:"target_address"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	Activated     bool   `json:"activated"`
}

// ListConnectionCodes 列出连接码
func (c *ManagementAPIClient) ListConnectionCodes() (*ListConnectionCodesResponse, error) {
	url := fmt.Sprintf("%s/tunnox/v1/connection-codes?client_id=%d", c.baseURL, c.clientID)

	respBody, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var resp ListConnectionCodesResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 隧道映射相关API
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ActivateCodeRequest 激活连接码请求
type ActivateCodeRequest struct {
	Code          string `json:"code"`
	ListenAddress string `json:"listen_address"` // 本地监听地址（如 127.0.0.1:8888）
}

// ActivateCodeResponse 激活连接码响应
type ActivateCodeResponse struct {
	MappingID     string `json:"mapping_id"`
	TargetAddress string `json:"target_address"`
	ListenAddress string `json:"listen_address"`
	ExpiresAt     string `json:"expires_at"`
}

// ActivateConnectionCode 激活连接码
func (c *ManagementAPIClient) ActivateConnectionCode(req *ActivateCodeRequest) (*ActivateCodeResponse, error) {
	url := fmt.Sprintf("%s/tunnox/v1/connection-codes/%s/activate", c.baseURL, req.Code)

	respBody, err := c.doRequest("POST", url, req)
	if err != nil {
		return nil, err
	}

	var resp ActivateCodeResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// ListMappingsResponse 映射列表响应
type ListMappingsResponse struct {
	Mappings []MappingInfo `json:"mappings"`
	Total    int           `json:"total"`
}

// MappingInfo 映射信息
type MappingInfo struct {
	MappingID     string `json:"mapping_id"`
	Type          string `json:"type"` // "inbound" or "outbound"
	TargetAddress string `json:"target_address"`
	ListenAddress string `json:"listen_address"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	UsageCount    int    `json:"usage_count"`
	BytesSent     int64  `json:"bytes_sent"`
	BytesReceived int64  `json:"bytes_received"`
}

// ListMappings 列出隧道映射
func (c *ManagementAPIClient) ListMappings(mappingType string) (*ListMappingsResponse, error) {
	url := fmt.Sprintf("%s/tunnox/v1/mappings?client_id=%d", c.baseURL, c.clientID)
	if mappingType != "" {
		url += fmt.Sprintf("&type=%s", mappingType)
	}

	respBody, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var resp ListMappingsResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// GetMapping 获取映射详情
func (c *ManagementAPIClient) GetMapping(mappingID string) (*MappingInfo, error) {
	url := fmt.Sprintf("%s/tunnox/v1/mappings/%s", c.baseURL, mappingID)

	respBody, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var resp MappingInfo
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// DeleteMapping 删除映射
func (c *ManagementAPIClient) DeleteMapping(mappingID string) error {
	url := fmt.Sprintf("%s/tunnox/v1/mappings/%s", c.baseURL, mappingID)

	_, err := c.doRequest("DELETE", url, nil)
	return err
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// doRequest 执行HTTP请求
func (c *ManagementAPIClient) doRequest(method, url string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

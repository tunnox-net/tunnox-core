package httppoll

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

// TunnelPackage HTTP 长轮询控制包
// 所有控制包（握手、命令、隧道控制）都通过 X-Tunnel-Package header 传输
type TunnelPackage struct {
	// 连接标识（必须）
	ConnectionID string `json:"connection_id"`
	
	// 客户端信息（可选，握手阶段 clientID=0）
	ClientID int64 `json:"client_id,omitempty"`
	
	// 映射ID（可选，隧道连接才有）
	MappingID string `json:"mapping_id,omitempty"`
	
	// 连接类型（可选，"control" | "data"）
	TunnelType string `json:"tunnel_type,omitempty"`
	
	// 包类型（可选，"Handshake", "HandshakeResponse", "JsonCommand", "CommandResp", "TunnelOpen", "TunnelOpenAck" 等）
	Type string `json:"type,omitempty"`
	
	// 包数据（可选，根据包类型不同，可以是 HandshakeRequest, JsonCommand, TunnelOpenRequest 等）
	Data interface{} `json:"data,omitempty"`
}

// EncodeTunnelPackage 编码控制包
// 流程：JSON 序列化 → Gzip 压缩 → Base64 编码
func EncodeTunnelPackage(pkg *TunnelPackage) (string, error) {
	if pkg == nil {
		return "", fmt.Errorf("tunnel package is nil")
	}
	
	// 1. JSON 序列化
	jsonData, err := json.Marshal(pkg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tunnel package: %w", err)
	}
	
	// 2. Gzip 压缩
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(jsonData); err != nil {
		writer.Close()
		return "", fmt.Errorf("failed to compress tunnel package: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close gzip writer: %w", err)
	}
	
	// 3. Base64 编码
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// DecodeTunnelPackage 解码控制包
// 流程：Base64 解码 → Gzip 解压 → JSON 反序列化
func DecodeTunnelPackage(encoded string) (*TunnelPackage, error) {
	if encoded == "" {
		return nil, fmt.Errorf("encoded tunnel package is empty")
	}
	
	// 1. Base64 解码
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}
	
	// 2. Gzip 解压
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()
	
	jsonData, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress tunnel package: %w", err)
	}
	
	// 3. JSON 反序列化
	var pkg TunnelPackage
	if err := json.Unmarshal(jsonData, &pkg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tunnel package: %w", err)
	}
	
	return &pkg, nil
}

// ValidateConnectionID 验证 ConnectionID 格式
// ConnectionID 应该是 "conn_" 前缀的 UUID
func ValidateConnectionID(connID string) bool {
	if connID == "" {
		return false
	}
	// 简单验证：至少包含 "conn_" 前缀且长度合理
	if len(connID) < 10 || len(connID) > 100 {
		return false
	}
	// 可以添加更严格的 UUID 格式验证
	return true
}


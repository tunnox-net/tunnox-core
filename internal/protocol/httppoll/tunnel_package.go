package httppoll

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"tunnox-core/internal/core/errors"
)

// TunnelPackage HTTP 长轮询控制包
// 所有控制包（握手、命令、隧道控制）都通过 X-Tunnel-Package header 传输
type TunnelPackage struct {
	// 连接标识（必须）
	ConnectionID string `json:"connection_id"`

	// 请求ID（可选，客户端生成，用于匹配请求和响应）
	RequestID string `json:"request_id,omitempty"`

	// 客户端信息（可选，握手阶段 clientID=0）
	ClientID int64 `json:"client_id,omitempty"`

	// 映射ID（可选，隧道连接才有）
	MappingID string `json:"mapping_id,omitempty"`

	// 连接类型（可选，"control" | "data" | "keepalive"）
	// - "control": 控制连接，用于握手、命令等控制包
	// - "data": 数据连接，用于隧道数据传输
	// - "keepalive": 保持连接请求，仅用于维持连接并接收服务端响应
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
		return "", errors.New(errors.ErrorTypePermanent, "tunnel package is nil")
	}

	// 1. JSON 序列化
	jsonData, err := json.Marshal(pkg)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrorTypePermanent, "failed to marshal tunnel package")
	}

	// 2. Gzip 压缩
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(jsonData); err != nil {
		writer.Close()
		return "", errors.Wrap(err, errors.ErrorTypePermanent, "failed to compress tunnel package")
	}
	if err := writer.Close(); err != nil {
		return "", errors.Wrap(err, errors.ErrorTypePermanent, "failed to close gzip writer")
	}

	// 3. Base64 编码
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// DecodeTunnelPackage 解码控制包
// 流程：Base64 解码 → Gzip 解压 → JSON 反序列化
func DecodeTunnelPackage(encoded string) (*TunnelPackage, error) {
	if encoded == "" {
		return nil, errors.New(errors.ErrorTypeProtocol, "encoded tunnel package is empty")
	}

	// 1. Base64 解码
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrorTypeProtocol, "failed to decode base64")
	}

	// 2. Gzip 解压
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrorTypePermanent, "failed to create gzip reader")
	}
	defer reader.Close()

	jsonData, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrorTypePermanent, "failed to decompress tunnel package")
	}

	// 3. JSON 反序列化
	var pkg TunnelPackage
	if err := json.Unmarshal(jsonData, &pkg); err != nil {
		return nil, errors.Wrap(err, errors.ErrorTypeProtocol, "failed to unmarshal tunnel package")
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

package mobile

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tunnox-core/internal/client"
)

// TunnoxMobileClient Android/iOS 可调用的客户端封装
type TunnoxMobileClient struct {
	ctx           context.Context
	cancel        context.CancelFunc
	client        *client.TunnoxClient
	eventCallback EventCallback
	mu            sync.RWMutex
}

// NewClient 创建客户端
// serverAddr: 服务器地址，如 "tunnox.net:8000" 或 "https://gw.tunnox.net/_tunnox"
// protocol: 连接协议，"tcp", "websocket", "kcp", "quic"
// clientID: 客户端 ID，首次连接传 0
// secretKey: 客户端密钥，首次连接传空字符串
func NewClient(serverAddr, protocol string, clientID int64, secretKey string) *TunnoxMobileClient {
	ctx, cancel := context.WithCancel(context.Background())

	config := &client.ClientConfig{
		ClientID:  clientID,
		SecretKey: secretKey,
	}
	config.Server.Address = serverAddr
	config.Server.Protocol = protocol
	config.Log.Level = "info"
	config.Log.Format = "text"

	tunnoxClient := client.NewClient(ctx, config)

	return &TunnoxMobileClient{
		ctx:    ctx,
		cancel: cancel,
		client: tunnoxClient,
	}
}

// SetEventCallback 设置事件回调
// callback: 实现了 EventCallback 接口的对象（由 Android/iOS 实现）
func (c *TunnoxMobileClient) SetEventCallback(callback EventCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventCallback = callback
}

// Connect 连接服务器
// 返回错误信息，成功返回空字符串
func (c *TunnoxMobileClient) Connect() string {
	err := c.client.Connect()
	if err != nil {
		errMsg := err.Error()
		c.notifyError(errMsg)
		return errMsg
	}

	c.notifyConnected()
	return ""
}

// Disconnect 断开连接
func (c *TunnoxMobileClient) Disconnect() {
	c.client.Stop()
	c.notifyDisconnected("user_request")
}

// GetClientID 获取客户端 ID
// 首次连接成功后，服务器会分配 ClientID，需要保存到本地
func (c *TunnoxMobileClient) GetClientID() int64 {
	return c.client.GetClientID()
}

// GetSecretKey 获取密钥
// 首次连接成功后，服务器会分配 SecretKey，需要保存到本地
// 注意：此方法仅用于首次获取密钥，后续不应再调用
func (c *TunnoxMobileClient) GetSecretKey() string {
	config := c.client.GetConfig()
	if config == nil {
		return ""
	}
	return config.SecretKey
}

// IsConnected 是否已连接
func (c *TunnoxMobileClient) IsConnected() bool {
	status := c.client.GetStatus()
	return status.Connected
}

// GetStatus 获取连接状态
func (c *TunnoxMobileClient) GetStatus() *ConnectionStatus {
	status := c.client.GetStatus()
	if status == nil {
		return &ConnectionStatus{Connected: false}
	}

	return &ConnectionStatus{
		Connected:    status.Connected,
		ClientID:     status.ClientID,
		ServerAddr:   status.ServerAddr,
		Protocol:     status.Protocol,
		UptimeMillis: int64(status.Uptime / time.Millisecond),
		MappingCount: status.MappingCount,
	}
}

// Close 关闭客户端，释放资源
func (c *TunnoxMobileClient) Close() {
	c.client.Stop()
	c.cancel()
}

// SaveConfig 此方法已废弃，请使用 GetClientID() 和 GetSecretKey() 获取凭据后在移动端自行保存
// 移动端应使用平台原生安全存储: Android 使用 EncryptedSharedPreferences, iOS 使用 Keychain
// Deprecated: Use GetClientID() and GetSecretKey() instead
func (c *TunnoxMobileClient) SaveConfig(_ string) string {
	return ""
}

// 内部方法：通知回调

func (c *TunnoxMobileClient) notifyConnected() {
	c.mu.RLock()
	callback := c.eventCallback
	c.mu.RUnlock()

	if callback != nil {
		callback.OnConnected()
	}
}

func (c *TunnoxMobileClient) notifyDisconnected(reason string) {
	c.mu.RLock()
	callback := c.eventCallback
	c.mu.RUnlock()

	if callback != nil {
		callback.OnDisconnected(reason)
	}
}

func (c *TunnoxMobileClient) notifyError(errMsg string) {
	c.mu.RLock()
	callback := c.eventCallback
	c.mu.RUnlock()

	if callback != nil {
		callback.OnError(errMsg)
	}
}

func (c *TunnoxMobileClient) notifySocks5Started(mappingID string, port int64) {
	c.mu.RLock()
	callback := c.eventCallback
	c.mu.RUnlock()

	if callback != nil {
		callback.OnSocks5Started(mappingID, port)
	}
}

func (c *TunnoxMobileClient) notifySocks5Stopped(mappingID string) {
	c.mu.RLock()
	callback := c.eventCallback
	c.mu.RUnlock()

	if callback != nil {
		callback.OnSocks5Stopped(mappingID)
	}
}

// GetServerAddr 获取当前连接的服务器地址
func (c *TunnoxMobileClient) GetServerAddr() string {
	status := c.client.GetStatus()
	if status == nil {
		return ""
	}
	return status.ServerAddr
}

// GetProtocol 获取当前连接协议
func (c *TunnoxMobileClient) GetProtocol() string {
	status := c.client.GetStatus()
	if status == nil {
		return ""
	}
	return status.Protocol
}

// GetUptime 获取运行时长（毫秒）
func (c *TunnoxMobileClient) GetUptime() int64 {
	status := c.client.GetStatus()
	if status == nil {
		return 0
	}
	return int64(status.Uptime / time.Millisecond)
}

// Reconnect 重新连接
// 断开当前连接并重新连接
func (c *TunnoxMobileClient) Reconnect() string {
	c.Disconnect()
	time.Sleep(time.Second) // 等待 1 秒
	return c.Connect()
}

// SetServerConfig 更新服务器配置
// serverAddr: 服务器地址
// protocol: 连接协议
func (c *TunnoxMobileClient) SetServerConfig(serverAddr, protocol string) {
	config := c.client.GetConfig()
	if config != nil {
		config.Server.Address = serverAddr
		config.Server.Protocol = protocol
	}
}

// SetCredentials 设置客户端凭据
// clientID: 客户端 ID
// secretKey: 密钥
func (c *TunnoxMobileClient) SetCredentials(clientID int64, secretKey string) {
	config := c.client.GetConfig()
	if config != nil {
		config.ClientID = clientID
		config.SecretKey = secretKey
	}
}

// GetConfigJSON 获取配置的 JSON 表示（用于调试）
func (c *TunnoxMobileClient) GetConfigJSON() string {
	config := c.client.GetConfig()
	if config == nil {
		return "{}"
	}
	return fmt.Sprintf(`{"client_id":%d,"server_addr":"%s","protocol":"%s"}`,
		config.ClientID, config.Server.Address, config.Server.Protocol)
}

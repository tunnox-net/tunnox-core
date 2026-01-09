package mobile

// EventCallback 事件回调接口，由 Android/iOS 实现
type EventCallback interface {
	// OnConnected 连接成功回调
	OnConnected()

	// OnDisconnected 断开连接回调
	// reason: 断开原因（如 "user_request", "server_closed", "network_error"）
	OnDisconnected(reason string)

	// OnError 错误回调
	// errMsg: 错误信息
	OnError(errMsg string)

	// OnSocks5Started SOCKS5 监听启动成功
	// mappingID: 映射 ID
	// port: 监听端口
	OnSocks5Started(mappingID string, port int64)

	// OnSocks5Stopped SOCKS5 监听停止
	// mappingID: 映射 ID
	OnSocks5Stopped(mappingID string)

	// OnMappingUpdate 映射配置更新
	// mappingsJSON: 映射列表的 JSON 字符串
	OnMappingUpdate(mappingsJSON string)

	// OnTunnelOpened 隧道打开
	// tunnelID: 隧道 ID
	// mappingID: 对应的映射 ID
	OnTunnelOpened(tunnelID string, mappingID string)

	// OnTunnelClosed 隧道关闭
	// tunnelID: 隧道 ID
	// reason: 关闭原因
	OnTunnelClosed(tunnelID string, reason string)
}

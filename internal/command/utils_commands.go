package command

import "tunnox-core/internal/packet"

// ==================== 连接管理类命令 ====================

// Connect 建立连接命令
func (cu *CommandUtils) Connect() *CommandUtils {
	return cu.WithCommand(packet.Connect)
}

// Reconnect 重新连接命令
func (cu *CommandUtils) Reconnect() *CommandUtils {
	return cu.WithCommand(packet.Reconnect)
}

// Disconnect 断开连接命令
func (cu *CommandUtils) Disconnect() *CommandUtils {
	return cu.WithCommand(packet.Disconnect)
}

// Heartbeat 心跳保活命令
func (cu *CommandUtils) Heartbeat() *CommandUtils {
	return cu.WithCommand(packet.HeartbeatCmd)
}

// ==================== 端口映射类命令 ====================

// TCP映射相关命令

// TcpMapCreate 创建TCP映射
func (cu *CommandUtils) TcpMapCreate() *CommandUtils {
	return cu.WithCommand(packet.TcpMapCreate)
}

// TcpMapDelete 删除TCP映射
func (cu *CommandUtils) TcpMapDelete() *CommandUtils {
	return cu.WithCommand(packet.TcpMapDelete)
}

// TcpMapUpdate 更新TCP映射
func (cu *CommandUtils) TcpMapUpdate() *CommandUtils {
	return cu.WithCommand(packet.TcpMapUpdate)
}

// TcpMapList 列出TCP映射
func (cu *CommandUtils) TcpMapList() *CommandUtils {
	return cu.WithCommand(packet.TcpMapList)
}

// TcpMapStatus 查询TCP映射状态
func (cu *CommandUtils) TcpMapStatus() *CommandUtils {
	return cu.WithCommand(packet.TcpMapStatus)
}

// HTTP映射相关命令

// HttpMapCreate 创建HTTP映射
func (cu *CommandUtils) HttpMapCreate() *CommandUtils {
	return cu.WithCommand(packet.HttpMapCreate)
}

// HttpMapDelete 删除HTTP映射
func (cu *CommandUtils) HttpMapDelete() *CommandUtils {
	return cu.WithCommand(packet.HttpMapDelete)
}

// HttpMapUpdate 更新HTTP映射
func (cu *CommandUtils) HttpMapUpdate() *CommandUtils {
	return cu.WithCommand(packet.HttpMapUpdate)
}

// HttpMapList 列出HTTP映射
func (cu *CommandUtils) HttpMapList() *CommandUtils {
	return cu.WithCommand(packet.HttpMapList)
}

// HttpMapStatus 查询HTTP映射状态
func (cu *CommandUtils) HttpMapStatus() *CommandUtils {
	return cu.WithCommand(packet.HttpMapStatus)
}

// SOCKS映射相关命令

// SocksMapCreate 创建SOCKS映射
func (cu *CommandUtils) SocksMapCreate() *CommandUtils {
	return cu.WithCommand(packet.SocksMapCreate)
}

// SocksMapDelete 删除SOCKS映射
func (cu *CommandUtils) SocksMapDelete() *CommandUtils {
	return cu.WithCommand(packet.SocksMapDelete)
}

// SocksMapUpdate 更新SOCKS映射
func (cu *CommandUtils) SocksMapUpdate() *CommandUtils {
	return cu.WithCommand(packet.SocksMapUpdate)
}

// SocksMapList 列出SOCKS映射
func (cu *CommandUtils) SocksMapList() *CommandUtils {
	return cu.WithCommand(packet.SocksMapList)
}

// SocksMapStatus 查询SOCKS映射状态
func (cu *CommandUtils) SocksMapStatus() *CommandUtils {
	return cu.WithCommand(packet.SocksMapStatus)
}

// ==================== 数据传输类命令 ====================

// DataTransferStart 开始数据传输
func (cu *CommandUtils) DataTransferStart() *CommandUtils {
	return cu.WithCommand(packet.DataTransferStart)
}

// DataTransferStop 停止数据传输
func (cu *CommandUtils) DataTransferStop() *CommandUtils {
	return cu.WithCommand(packet.DataTransferStop)
}

// DataTransferStatus 查询数据传输状态
func (cu *CommandUtils) DataTransferStatus() *CommandUtils {
	return cu.WithCommand(packet.DataTransferStatus)
}

// ProxyForward 代理转发
func (cu *CommandUtils) ProxyForward() *CommandUtils {
	return cu.WithCommand(packet.ProxyForward)
}

// ==================== 系统管理类命令 ====================

// ConfigGet 获取配置
func (cu *CommandUtils) ConfigGet() *CommandUtils {
	return cu.WithCommand(packet.ConfigGet)
}

// ConfigSet 设置配置
func (cu *CommandUtils) ConfigSet() *CommandUtils {
	return cu.WithCommand(packet.ConfigSet)
}

// StatsGet 获取统计信息
func (cu *CommandUtils) StatsGet() *CommandUtils {
	return cu.WithCommand(packet.StatsGet)
}

// LogGet 获取日志
func (cu *CommandUtils) LogGet() *CommandUtils {
	return cu.WithCommand(packet.LogGet)
}

// HealthCheck 健康检查
func (cu *CommandUtils) HealthCheck() *CommandUtils {
	return cu.WithCommand(packet.HealthCheck)
}

// ==================== RPC类命令 ====================

// RpcInvoke 调用RPC
func (cu *CommandUtils) RpcInvoke() *CommandUtils {
	return cu.WithCommand(packet.RpcInvoke)
}

// RpcRegister 注册RPC
func (cu *CommandUtils) RpcRegister() *CommandUtils {
	return cu.WithCommand(packet.RpcRegister)
}

// RpcUnregister 注销RPC
func (cu *CommandUtils) RpcUnregister() *CommandUtils {
	return cu.WithCommand(packet.RpcUnregister)
}

// RpcList 列出RPC
func (cu *CommandUtils) RpcList() *CommandUtils {
	return cu.WithCommand(packet.RpcList)
}

// ==================== 兼容性命令（保留原有方法） ====================

// TcpMap 便捷方法：创建TCP映射命令（兼容性）
func (cu *CommandUtils) TcpMap() *CommandUtils {
	return cu.WithCommand(packet.TcpMapCreate)
}

// HttpMap 便捷方法：创建HTTP映射命令（兼容性）
func (cu *CommandUtils) HttpMap() *CommandUtils {
	return cu.WithCommand(packet.HttpMapCreate)
}

// SocksMap 便捷方法：创建SOCKS映射命令（兼容性）
func (cu *CommandUtils) SocksMap() *CommandUtils {
	return cu.WithCommand(packet.SocksMapCreate)
}

// DataIn 便捷方法：创建数据输入命令（兼容性）
func (cu *CommandUtils) DataIn() *CommandUtils {
	return cu.WithCommand(packet.DataTransferStart)
}

// Forward 便捷方法：创建转发命令（兼容性）
func (cu *CommandUtils) Forward() *CommandUtils {
	return cu.WithCommand(packet.ProxyForward)
}

// DataOut 便捷方法：创建数据输出命令（兼容性）
func (cu *CommandUtils) DataOut() *CommandUtils {
	return cu.WithCommand(packet.DataTransferOut)
}

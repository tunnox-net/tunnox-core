package protocol

import (
	"fmt"
	"io"
	"sync"
	"tunnox-core/internal/cloud"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// ConnectionSession用于统一处理业务逻辑
// 将所有的协议通过AcceptConnection做成conn / ClientID 映射，并关联PackageStreamer，
// 后需就都只基于并关联PackageStreamer 操作流

type ConnectionSession struct {
	cloudApi cloud.CloudControlAPI
	connMap  map[io.Reader]string
	streamer map[string]stream.PackageStreamer

	connMapLock  sync.RWMutex
	streamerLock sync.RWMutex

	utils.Dispose
}

func (s *ConnectionSession) dispatchTransfer(streamer *stream.PackageStream, connID string) {
	utils.Info("Starting dispatch transfer for connection", connID)
	defer func() {
		utils.Info("Dispatch transfer ended for connection", connID)
		// 确保流被关闭
		streamer.Close()
	}()

	// 持续读取数据包，直到出错或连接关闭
	for {
		// 检查上下文是否已取消
		select {
		case <-s.Ctx().Done():
			utils.Info("Context cancelled, stopping dispatch transfer for connection", connID)
			return
		default:
		}

		// 读取数据包
		transferPacket, bytesRead, err := streamer.ReadPacket()
		if err != nil {
			if err == io.EOF {
				utils.Info("Connection closed by peer for connection", connID)
			} else {
				utils.Error("Failed to read packet for connection", connID, "error:", err)
			}
			return
		}

		utils.Debug("Read packet for connection", connID, "bytes:", bytesRead, "type:", transferPacket.PacketType)

		// 处理不同类型的数据包
		if err := s.processPacket(transferPacket, streamer, connID); err != nil {
			utils.Error("Failed to process packet for connection", connID, "error:", err)
			return
		}
	}
}

func (s *ConnectionSession) processPacket(transferPacket *packet.TransferPacket, streamer *stream.PackageStream, connID string) error {
	// 处理心跳包
	if transferPacket.PacketType.IsHeartbeat() {
		return s.handleHeartbeat(transferPacket, streamer, connID)
	}

	// 处理JSON命令包
	if transferPacket.PacketType.IsJsonCommand() && transferPacket.CommandPacket != nil {
		return s.handleCommandPacket(transferPacket.CommandPacket, streamer, connID)
	}

	// 处理其他类型的包
	utils.Warn("Unsupported packet type for connection", connID, "type:", transferPacket.PacketType)
	return nil
}

func (s *ConnectionSession) handleHeartbeat(transferPacket *packet.TransferPacket, streamer *stream.PackageStream, connID string) error {
	utils.Debug("Processing heartbeat for connection", connID)

	// TODO: 实现心跳处理逻辑
	// - 更新连接状态
	// - 记录心跳时间
	// - 可选：发送心跳响应

	return nil
}

func (s *ConnectionSession) handleCommandPacket(commandPacket *packet.CommandPacket, streamer *stream.PackageStream, connID string) error {
	utils.Info("Processing command packet for connection", connID, "command:", commandPacket.CommandType)

	// 根据命令类型分发处理
	switch commandPacket.CommandType {
	case packet.TcpMap:
		return s.handleTcpMapCommand(commandPacket, streamer, connID)
	case packet.HttpMap:
		return s.handleHttpMapCommand(commandPacket, streamer, connID)
	case packet.SocksMap:
		return s.handleSocksMapCommand(commandPacket, streamer, connID)
	case packet.DataIn:
		return s.handleDataInCommand(commandPacket, streamer, connID)
	case packet.Forward:
		return s.handleForwardCommand(commandPacket, streamer, connID)
	case packet.DataOut:
		return s.handleDataOutCommand(commandPacket, streamer, connID)
	case packet.Disconnect:
		return s.handleDisconnectCommand(commandPacket, streamer, connID)
	default:
		utils.Warn("Unknown command type for connection", connID, "command:", commandPacket.CommandType)
		return nil
	}
}

// TODO: 实现各种命令处理函数
func (s *ConnectionSession) handleTcpMapCommand(commandPacket *packet.CommandPacket, streamer *stream.PackageStream, connID string) error {
	utils.Info("TODO: Handle TCP mapping command for connection", connID)
	// TODO: 实现TCP端口映射逻辑
	// - 解析映射参数
	// - 创建本地监听端口
	// - 建立数据转发通道
	return nil
}

func (s *ConnectionSession) handleHttpMapCommand(commandPacket *packet.CommandPacket, streamer *stream.PackageStream, connID string) error {
	utils.Info("TODO: Handle HTTP mapping command for connection", connID)
	// TODO: 实现HTTP端口映射逻辑
	// - 解析HTTP映射参数
	// - 创建HTTP代理服务
	// - 处理HTTP请求转发
	return nil
}

func (s *ConnectionSession) handleSocksMapCommand(commandPacket *packet.CommandPacket, streamer *stream.PackageStream, connID string) error {
	utils.Info("TODO: Handle SOCKS mapping command for connection", connID)
	// TODO: 实现SOCKS代理映射逻辑
	// - 解析SOCKS映射参数
	// - 创建SOCKS代理服务
	// - 处理SOCKS协议
	return nil
}

func (s *ConnectionSession) handleDataInCommand(commandPacket *packet.CommandPacket, streamer *stream.PackageStream, connID string) error {
	utils.Info("TODO: Handle DataIn command for connection", connID)
	// TODO: 实现数据输入处理逻辑
	// - 处理客户端TCP监听端口收到的新连接
	// - 通知服务端准备透传
	// - 建立数据传输通道
	return nil
}

func (s *ConnectionSession) handleForwardCommand(commandPacket *packet.CommandPacket, streamer *stream.PackageStream, connID string) error {
	utils.Info("TODO: Handle Forward command for connection", connID)
	// TODO: 实现服务端间转发逻辑
	// - 检测需要中转的连接
	// - 通知其他服务端准备透传
	// - 建立跨服务端数据通道
	return nil
}

func (s *ConnectionSession) handleDataOutCommand(commandPacket *packet.CommandPacket, streamer *stream.PackageStream, connID string) error {
	utils.Info("TODO: Handle DataOut command for connection", connID)
	// TODO: 实现数据输出处理逻辑
	// - 服务端通知目标客户端准备透传
	// - 建立客户端间数据通道
	// - 处理数据转发
	return nil
}

func (s *ConnectionSession) handleDisconnectCommand(commandPacket *packet.CommandPacket, streamer *stream.PackageStream, connID string) error {
	utils.Info("TODO: Handle Disconnect command for connection", connID)
	// TODO: 实现连接断开处理逻辑
	// - 清理连接资源
	// - 通知相关组件
	// - 关闭数据传输通道
	return nil
}

func (s *ConnectionSession) AcceptConnection(reader io.Reader, writer io.Writer) {
	// 1. 通过云控注册/鉴权，获取 connId
	connId := ""
	if s.cloudApi != nil {
		// 这里假设通过 Authenticate 获取 connId（可根据实际业务替换）
		req := &cloud.AuthRequest{
			// 填充必要的鉴权信息，如 ClientID、AuthCode、SecretKey、NodeID、Version、IPAddress、Type
		}
		resp, err := s.cloudApi.Authenticate(req)
		if err != nil || resp == nil || resp.Client == nil {
			utils.Error("Cloud authentication failed for new connection:", err)
			return
		}
		connId = fmt.Sprintf("%d", resp.Client.ID)
	} else {
		utils.Warn("cloudApi is nil, cannot get connId from cloud, using fallback id")
		connId = "unknown"
	}

	// 2. 写入映射
	s.connMapLock.Lock()
	s.connMap[reader] = connId
	s.connMapLock.Unlock()

	// 3. 传递 connId 给 dispatchTransfer
	ps := stream.NewPackageStream(reader, writer, s.Ctx())
	ps.AddCloseFunc(func() {
		s.connMapLock.Lock()
		defer s.connMapLock.Unlock()
		delete(s.connMap, reader)
	})
	go s.dispatchTransfer(ps, connId)
}

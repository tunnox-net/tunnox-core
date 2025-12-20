package api

import (
corelog "tunnox-core/internal/core/log"
	"encoding/json"
	"fmt"
	"time"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/utils"
)

// handleControlPackage 处理控制包
func (s *ManagementAPIServer) handleControlPackage(streamProcessor *httppoll.ServerStreamProcessor, pkg *httppoll.TunnelPackage) *httppoll.TunnelPackage {
	connID := streamProcessor.GetConnectionID()
	corelog.Infof("HTTP long polling: handleControlPackage - processing package, type=%s, connID=%s", pkg.Type, connID)

	if s.sessionMgr == nil {
		corelog.Debugf("HTTP long polling: handleControlPackage - sessionMgr is nil, connID=%s", connID)
		return nil
	}

	// 获取连接对应的 Connection 对象
	// 注意：对于握手包，连接可能还没有在 SessionManager 中注册，需要先创建连接
	corelog.Infof("HTTP long polling: handleControlPackage - getting sessionMgrWithConn, connID=%s, pkgType=%s", connID, pkg.Type)
	sessionMgrWithConn := getSessionManagerWithConnection(s.sessionMgr)
	if sessionMgrWithConn == nil {
		corelog.Errorf("HTTP long polling: handleControlPackage - sessionMgrWithConn is nil, connID=%s, pkgType=%s", connID, pkg.Type)
		return nil
	}
	corelog.Infof("HTTP long polling: handleControlPackage - sessionMgrWithConn obtained, connID=%s, pkgType=%s", connID, pkg.Type)

	typesConn, exists := sessionMgrWithConn.GetConnection(connID)
	corelog.Infof("HTTP long polling: handleControlPackage - GetConnection result, connID=%s, exists=%v, typesConn=%v, pkgType=%s", connID, exists, typesConn != nil, pkg.Type)
	if !exists || typesConn == nil {
		// 对于握手包，连接可能还没有创建，这是正常的
		// 让 HandlePacket 来处理连接的创建
		corelog.Infof("HTTP long polling: connection not found in SessionManager (may be created during handshake), connID=%s, pkgType=%s", connID, pkg.Type)
		// 不返回 nil，继续处理握手包
	}

	// 根据包类型处理
	corelog.Infof("HTTP long polling: handleControlPackage - switching on pkgType=%s, connID=%s", pkg.Type, connID)
	switch pkg.Type {
	case "Handshake":
		corelog.Infof("HTTP long polling: calling handleHandshakePackage, connID=%s", connID)
		return s.handleHandshakePackage(streamProcessor, pkg, typesConn)
	case "JsonCommand":
		corelog.Infof("HTTP long polling: handleControlPackage - processing JsonCommand, connID=%s", connID)
		result := s.handleJsonCommandPackage(streamProcessor, pkg, typesConn)
		corelog.Infof("HTTP long polling: handleControlPackage - JsonCommand processed, result=%v, connID=%s", result != nil, connID)
		return result
	case "TunnelOpen":
		return s.handleTunnelOpenPackage(streamProcessor, pkg, typesConn)
	default:
		corelog.Warnf("HTTP long polling: unknown control package type: %s", pkg.Type)
		return nil
	}
}

// handleHandshakePackage 处理握手包
// 注意：typesConn 可能为 nil（握手时连接尚未创建），这是正常的
func (s *ManagementAPIServer) handleHandshakePackage(streamProcessor *httppoll.ServerStreamProcessor, pkg *httppoll.TunnelPackage, typesConn *types.Connection) *httppoll.TunnelPackage {
	corelog.Infof("HTTP long polling: handleHandshakePackage called, connID=%s, typesConn=%v", streamProcessor.GetConnectionID(), typesConn != nil)
	// 解析 HandshakeRequest
	dataBytes, err := json.Marshal(pkg.Data)
	if err != nil {
		corelog.Errorf("HTTP long polling: failed to marshal handshake data: %v", err)
		return nil
	}

	var handshakeReq packet.HandshakeRequest
	if err := json.Unmarshal(dataBytes, &handshakeReq); err != nil {
		corelog.Errorf("HTTP long polling: failed to unmarshal handshake request: %v", err)
		return nil
	}

	// 获取 ConnectionID（应该已经由 createHTTPLongPollingConnection 生成）
	connID := streamProcessor.GetConnectionID()
	if connID == "" {
		// 如果还没有 ConnectionID，生成一个
		uuid, err := utils.GenerateUUID()
		if err != nil {
			corelog.Errorf("HTTP long polling: failed to generate connection ID: %v", err)
			return &httppoll.TunnelPackage{
				Type: "HandshakeResponse",
				Data: &packet.HandshakeResponse{
					Success: false,
					Error:   fmt.Sprintf("failed to generate connection ID: %v", err),
				},
			}
		}
		connID = "conn_" + uuid[:8]
		streamProcessor.SetConnectionID(connID)
		corelog.Infof("HTTP long polling: generated connection ID: %s", connID)
	}

	// 构造 StreamPacket
	streamPacket := &types.StreamPacket{
		ConnectionID: connID,
		Packet: &packet.TransferPacket{
			PacketType: packet.Handshake,
			Payload:    dataBytes,
		},
		Timestamp: time.Now(),
	}

	// 处理数据包（通过 SessionManager）
	// HandlePacket 会调用 sendHandshakeResponse，将响应通过 WritePacket 放入队列
	// 响应会通过 Poll 请求返回给客户端，不需要在这里直接返回
	handler, ok := s.sessionMgr.(interface {
		HandlePacket(*types.StreamPacket) error
	})
	if !ok {
		corelog.Errorf("HTTP long polling: SessionManager does not support HandlePacket, connID=%s", connID)
		return &httppoll.TunnelPackage{
			ConnectionID: connID,
			Type:         "HandshakeResponse",
			Data: &packet.HandshakeResponse{
				Success: false,
				Error:   "SessionManager does not support HandlePacket",
			},
		}
	}

	corelog.Infof("HTTP long polling: calling HandlePacket for handshake, connID=%s", connID)
		if err := handler.HandlePacket(streamPacket); err != nil {
		corelog.Errorf("HTTP long polling: failed to handle handshake packet: %v, connID=%s", err, connID)
		// 错误情况下，返回错误响应（通过 Push 响应立即返回）
			return &httppoll.TunnelPackage{
				ConnectionID: connID,
				Type:         "HandshakeResponse",
				Data: &packet.HandshakeResponse{
					Success: false,
					Error:   err.Error(),
				},
			}
		}
	// 成功处理：握手响应已通过 WritePacket 放入队列，将通过 Poll 请求返回
	// 不需要在这里返回响应，避免重复响应
	corelog.Infof("HTTP long polling: handshake packet handled successfully, response will be sent via Poll request, connID=%s", connID)
	return nil
}

// getControlConnectionByConnID 通过 ConnectionID 获取控制连接
// 注意：此方法目前未使用，保留用于未来扩展
func (s *ManagementAPIServer) getControlConnectionByConnID(connID string) ControlConnectionAccessor {
	// 通过 SessionManager 获取控制连接
	// 注意：SessionManager 接口目前没有按 ConnID 查找的方法
	// 需要通过 clientID 查找，这里暂时返回 nil
	// 未来可以扩展 SessionManager 接口添加 GetControlConnectionByConnID 方法
	return nil
}

// handleJsonCommandPackage 处理 JSON 命令包
func (s *ManagementAPIServer) handleJsonCommandPackage(streamProcessor *httppoll.ServerStreamProcessor, pkg *httppoll.TunnelPackage, typesConn *types.Connection) *httppoll.TunnelPackage {
	connID := streamProcessor.GetConnectionID()
	processStartTime := time.Now()

	// [CMD_TRACE] 服务端接收命令开始
	corelog.Infof("[CMD_TRACE] [SERVER] [RECV_START] ConnID=%s, RequestID=%s, Time=%s",
		connID, pkg.RequestID, processStartTime.Format("15:04:05.000"))

	// 使用 TunnelPackageToTransferPacket 正确解析 CommandPacket
	transferPkt, err := httppoll.TunnelPackageToTransferPacket(pkg)
	if err != nil {
		corelog.Errorf("[CMD_TRACE] [SERVER] [RECV_FAILED] ConnID=%s, RequestID=%s, Error=%v, Time=%s",
			connID, pkg.RequestID, err, time.Now().Format("15:04:05.000"))
		return nil
	}

	// 确保 CommandPacket 存在
	if transferPkt.CommandPacket == nil {
		corelog.Errorf("[CMD_TRACE] [SERVER] [RECV_FAILED] ConnID=%s, RequestID=%s, Error=CommandPacket_is_nil, Time=%s",
			connID, pkg.RequestID, time.Now().Format("15:04:05.000"))
		return nil
	}

	commandID := transferPkt.CommandPacket.CommandId
	commandType := transferPkt.CommandPacket.CommandType
	corelog.Infof("[CMD_TRACE] [SERVER] [RECV_COMPLETE] ConnID=%s, RequestID=%s, CommandID=%s, CommandType=%d, RecvDuration=%v, Time=%s",
		connID, pkg.RequestID, commandID, commandType, time.Since(processStartTime), time.Now().Format("15:04:05.000"))

	// 构造 StreamPacket（包含 CommandPacket）
	streamPacket := &types.StreamPacket{
		ConnectionID: connID,
		Packet:       transferPkt,
		Timestamp:    time.Now(),
	}

	// 处理数据包（通过 SessionManager）
	handleStartTime := time.Now()
	if handler, ok := s.sessionMgr.(interface {
		HandlePacket(*types.StreamPacket) error
	}); ok {
		corelog.Infof("[CMD_TRACE] [SERVER] [HANDLE_START] ConnID=%s, RequestID=%s, CommandID=%s, Time=%s",
			connID, pkg.RequestID, commandID, handleStartTime.Format("15:04:05.000"))
		if err := handler.HandlePacket(streamPacket); err != nil {
			corelog.Errorf("[CMD_TRACE] [SERVER] [HANDLE_FAILED] ConnID=%s, RequestID=%s, CommandID=%s, Error=%v, HandleDuration=%v, Time=%s",
				connID, pkg.RequestID, commandID, err, time.Since(handleStartTime), time.Now().Format("15:04:05.000"))
			return nil
		}
		handleDuration := time.Since(handleStartTime)
		corelog.Infof("[CMD_TRACE] [SERVER] [HANDLE_COMPLETE] ConnID=%s, RequestID=%s, CommandID=%s, HandleDuration=%v, TotalDuration=%v, Time=%s",
			connID, pkg.RequestID, commandID, handleDuration, time.Since(processStartTime), time.Now().Format("15:04:05.000"))
	} else {
		corelog.Warnf("[CMD_TRACE] [SERVER] [HANDLE_FAILED] ConnID=%s, RequestID=%s, CommandID=%s, Error=sessionMgr_does_not_implement_HandlePacket, Time=%s",
			connID, pkg.RequestID, commandID, time.Now().Format("15:04:05.000"))
	}

	// 命令响应通过 Poll 获取
	corelog.Infof("[CMD_TRACE] [SERVER] [RECV_END] ConnID=%s, RequestID=%s, CommandID=%s, ResponseVia=Poll, Time=%s",
		connID, pkg.RequestID, commandID, time.Now().Format("15:04:05.000"))
	return nil
}

// handleTunnelOpenPackage 处理隧道打开包
func (s *ManagementAPIServer) handleTunnelOpenPackage(streamProcessor *httppoll.ServerStreamProcessor, pkg *httppoll.TunnelPackage, typesConn *types.Connection) *httppoll.TunnelPackage {
	// 解析 TunnelOpenRequest
	dataBytes, err := json.Marshal(pkg.Data)
	if err != nil {
		corelog.Errorf("HTTP long polling: failed to marshal tunnel open data: %v", err)
		return nil
	}

	var tunnelOpenReq packet.TunnelOpenRequest
	if err := json.Unmarshal(dataBytes, &tunnelOpenReq); err != nil {
		corelog.Errorf("HTTP long polling: failed to unmarshal tunnel open request: %v", err)
		return nil
	}

	// 设置 mappingID
	if tunnelOpenReq.MappingID != "" {
		streamProcessor.SetMappingID(tunnelOpenReq.MappingID)
	}

	// 构造 StreamPacket
	streamPacket := &types.StreamPacket{
		ConnectionID: streamProcessor.GetConnectionID(),
		Packet: &packet.TransferPacket{
			PacketType: packet.TunnelOpen,
			Payload:    dataBytes,
		},
		Timestamp: time.Now(),
	}

	// 处理数据包（通过 SessionManager）
	if handler, ok := s.sessionMgr.(interface {
		HandlePacket(*types.StreamPacket) error
	}); ok {
		if err := handler.HandlePacket(streamPacket); err != nil {
			corelog.Errorf("HTTP long polling: failed to handle tunnel open packet: %v", err)
			return &httppoll.TunnelPackage{
				Type: "TunnelOpenAck",
				Data: &packet.TunnelOpenAckResponse{
					TunnelID: tunnelOpenReq.TunnelID,
					Success:  false,
					Error:    err.Error(),
				},
			}
		}
	}

	// TunnelOpenAck 响应通过 Poll 获取
	return nil
}

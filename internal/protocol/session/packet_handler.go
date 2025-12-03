package session

import (
	"encoding/json"
	"fmt"
	"net"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// ============================================================================
// 数据包处理
// ============================================================================

// ProcessPacket 处理数据包（兼容旧接口）
func (s *SessionManager) ProcessPacket(connID string, pkt *packet.TransferPacket) error {
	// 转换为 StreamPacket
	streamPacket := &types.StreamPacket{
		ConnectionID: connID,
		Packet:       pkt,
	}

	return s.HandlePacket(streamPacket)
}

// HandlePacket 处理数据包（统一入口）
func (s *SessionManager) HandlePacket(connPacket *types.StreamPacket) error {
	if connPacket == nil || connPacket.Packet == nil {
		return fmt.Errorf("invalid packet: nil")
	}

	packetType := connPacket.Packet.PacketType

	// 根据数据包类型分发（忽略压缩/加密标志）
	switch {
	case packetType.IsJsonCommand() || packetType.IsCommandResp():
		return s.handleCommandPacket(connPacket)

	case packetType&0x3F == packet.Handshake:
		return s.handleHandshake(connPacket)

	case packetType&0x3F == packet.TunnelOpen:
		return s.handleTunnelOpen(connPacket)

	case packetType.IsHeartbeat():
		return s.handleHeartbeat(connPacket)

	default:
		utils.Warnf("Unhandled packet type: %v", packetType)
		return fmt.Errorf("unhandled packet type: %v", packetType)
	}
}

// handleHandshake 处理握手请求
func (s *SessionManager) handleHandshake(connPacket *types.StreamPacket) error {
	if s.authHandler == nil {
		return fmt.Errorf("auth handler not configured")
	}

	// 解析握手请求（从 Payload）
	req := &packet.HandshakeRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			utils.Errorf("Failed to parse handshake request: %v", err)
			return fmt.Errorf("invalid handshake request format: %w", err)
		}
	}

	isControlConnection := req.ConnectionType != "tunnel"
	if req.ConnectionType == "" {
		isControlConnection = true
	}

	var clientConn ControlConnectionInterface
	if isControlConnection {
		// 获取或创建控制连接
		existingConn := s.getControlConnectionByConnID(connPacket.ConnectionID)
		if existingConn != nil {
			clientConn = existingConn
		} else {
			// 获取底层连接信息
			conn := s.getConnectionByConnID(connPacket.ConnectionID)
			if conn == nil {
				return fmt.Errorf("connection not found: %s", connPacket.ConnectionID)
			}

			// 创建控制连接
			enforcedProtocol := conn.Protocol
			if enforcedProtocol == "" {
				enforcedProtocol = "tcp" // 默认协议
			}
			// 从连接中提取远程地址
			var remoteAddr net.Addr
			if conn.RawConn != nil {
				remoteAddr = conn.RawConn.RemoteAddr()
			}
			newConn := NewControlConnection(conn.ID, conn.Stream, remoteAddr, enforcedProtocol)
			s.RegisterControlConnection(newConn)
			clientConn = newConn
		}
	} else {
		conn := s.getConnectionByConnID(connPacket.ConnectionID)
		if conn == nil {
			return fmt.Errorf("connection not found: %s", connPacket.ConnectionID)
		}
		enforcedProtocol := conn.Protocol
		if enforcedProtocol == "" {
			enforcedProtocol = "tcp"
		}
		var remoteAddr net.Addr
		if conn.RawConn != nil {
			remoteAddr = conn.RawConn.RemoteAddr()
		}
		newConn := NewControlConnection(conn.ID, conn.Stream, remoteAddr, enforcedProtocol)
		s.controlConnLock.Lock()
		s.controlConnMap[connPacket.ConnectionID] = newConn
		s.controlConnLock.Unlock()
		clientConn = newConn
	}

	// 调用 authHandler 处理
	resp, err := s.authHandler.HandleHandshake(clientConn, req)
	if err != nil {
		utils.Errorf("Handshake failed for connection %s: %v", connPacket.ConnectionID, err)
		// 发送失败响应
		s.sendHandshakeResponse(clientConn, &packet.HandshakeResponse{
			Success: false,
			Error:   err.Error(),
		})
		return err
	}

	// 发送成功响应
	if err := s.sendHandshakeResponse(clientConn, resp); err != nil {
		utils.Errorf("Failed to send handshake response: %v", err)
		return err
	}

	if isControlConnection && clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
		s.controlConnLock.Lock()
		oldConn, exists := s.clientIDIndexMap[clientConn.GetClientID()]
		if exists && oldConn != nil && oldConn.GetConnID() != clientConn.GetConnID() {
			utils.Warnf("Client %d reconnected: oldConnID=%s, newConnID=%s, cleaning up old connection",
				clientConn.GetClientID(), oldConn.GetConnID(), clientConn.GetConnID())
			delete(s.controlConnMap, oldConn.GetConnID())
		}
		// 需要将接口转换为具体类型存储（内部实现需要）
		if concreteConn, ok := clientConn.(*ControlConnection); ok {
			s.clientIDIndexMap[clientConn.GetClientID()] = concreteConn
		}
		s.controlConnLock.Unlock()

		conn := s.getConnectionByConnID(connPacket.ConnectionID)
		if conn != nil && conn.Stream != nil {
			// 使用接口获取 Reader，而不是类型断言
			reader := conn.Stream.GetReader()

			// 协议特定的握手后处理（通过统一的回调接口）
			if handshakeHandler, ok := reader.(interface{ OnHandshakeComplete(clientID int64) }); ok {
				handshakeHandler.OnHandshakeComplete(clientConn.GetClientID())
			}
		}
	}

	utils.Infof("Handshake succeeded for connection %s, ClientID=%d",
		connPacket.ConnectionID, clientConn.GetClientID())
	return nil
}

// handleTunnelOpen 处理隧道打开请求
// 这个方法处理两种情况：
// 1. 源端客户端发起的隧道连接（需要创建bridge并通知目标端）
// 2. 目标端客户端响应的隧道连接（连接到已有的bridge）
func (s *SessionManager) handleTunnelOpen(connPacket *types.StreamPacket) error {
	if s.tunnelHandler == nil {
		return fmt.Errorf("tunnel handler not configured")
	}

	// 获取底层连接
	conn := s.getConnectionByConnID(connPacket.ConnectionID)
	if conn == nil {
		return fmt.Errorf("connection not found: %s", connPacket.ConnectionID)
	}

	// 解析隧道打开请求（从 Payload）
	req := &packet.TunnelOpenRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			utils.Errorf("Failed to parse tunnel open request: %v", err)
			s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
				TunnelID: "",
				Success:  false,
				Error:    fmt.Sprintf("invalid tunnel open request format: %v", err),
			})
			return fmt.Errorf("invalid tunnel open request format: %w", err)
		}
	}

	utils.Infof("Tunnel open request: TunnelID=%s, MappingID=%s, ConnID=%s",
		req.TunnelID, req.MappingID, connPacket.ConnectionID)

	// ✅ 对于支持 mappingID 的连接，立即设置 mappingID 并注册隧道连接
	// 这样后续的请求就能正确路由到隧道连接
	if req.MappingID != "" && conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if mappingConn, ok := reader.(interface {
			GetClientID() int64
			SetMappingID(mappingID string)
		}); ok {
			clientID := mappingConn.GetClientID()
			if clientID > 0 {
				utils.Infof("Tunnel[%s]: setting mappingID immediately for tunnel connection, MappingID=%s, ConnID=%s, ClientID=%d",
					req.TunnelID, req.MappingID, connPacket.ConnectionID, clientID)
				mappingConn.SetMappingID(req.MappingID)
			}
		}
	}

	// 检查是否已有bridge（目标端连接或源端重连）
	s.bridgeLock.Lock()
	bridge, exists := s.tunnelBridges[req.TunnelID]
	s.bridgeLock.Unlock()

	if exists {
		// 这是目标端的连接，或源端重连（HTTP 长轮询每次都创建新连接）

		// ✅ 隧道连接（有 MappingID）不应该被注册为控制连接
		// 由于现在在 Handshake 中已经通过 ConnectionType 识别，这里只需要清理可能的误注册
		if req.MappingID != "" {
			s.controlConnLock.Lock()
			if controlConn, exists := s.controlConnMap[connPacket.ConnectionID]; exists {
				// 如果这个连接被错误注册为控制连接，移除它
				delete(s.controlConnMap, connPacket.ConnectionID)
				if controlConn.IsAuthenticated() && controlConn.GetClientID() > 0 {
					// 如果 clientIDIndexMap 中也指向这个连接，移除它
					if currentControlConn, exists := s.clientIDIndexMap[controlConn.ClientID]; exists && currentControlConn.ConnID == connPacket.ConnectionID {
						delete(s.clientIDIndexMap, controlConn.ClientID)
					}
				}
			}
			s.controlConnLock.Unlock()
		}

		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  true,
		})

		netConn := s.extractNetConn(conn)
		// 如果无法提取 net.Conn，尝试从 Stream 创建数据转发器（通过接口抽象）
		if netConn == nil && conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			writer := conn.Stream.GetWriter()
			if reader == nil || writer == nil {
				// 该协议不支持桥接（如 HTTP 长轮询），数据已通过协议本身传输
				utils.Infof("Tunnel[%s]: connection does not support net.Conn bridge, data forwarding handled by protocol", req.TunnelID)
			}
		}

		// ✅ 判断是源端还是目标端连接，更新对应的连接
		// 通过 cloudControl 获取映射配置，判断 clientID 是源端还是目标端
		netConn = s.extractNetConn(conn)
		var isSourceClient bool
		if s.cloudControl != nil && req.MappingID != "" {
			mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
			if err == nil {
				listenClientID := mapping.ListenClientID
				if listenClientID == 0 {
					listenClientID = mapping.SourceClientID
				}
				// 从连接中获取 clientID（使用 extractClientID 函数，支持多种方式）
				connClientID := extractClientID(conn.Stream, netConn)
				// 如果 extractClientID 返回 0，稍后从控制连接获取（clientConn 在后面定义）
				isSourceClient = (connClientID == listenClientID)
				utils.Infof("Tunnel[%s]: identified connection type - isSourceClient=%v, connClientID=%d, listenClientID=%d, targetClientID=%d",
					req.TunnelID, isSourceClient, connClientID, listenClientID, mapping.TargetClientID)
			}
		}

		// 创建统一接口连接
		clientID := extractClientID(conn.Stream, netConn)
		tunnelConn := CreateTunnelConnection(conn.ID, netConn, conn.Stream, clientID, req.MappingID, req.TunnelID)

		if isSourceClient {
			// 源端重连，更新 sourceConn
			bridge.SetSourceConnection(tunnelConn)
			utils.Infof("Tunnel[%s]: updated sourceConn for existing bridge, connID=%s, hasNetConn=%v", req.TunnelID, conn.ID, netConn != nil)
		} else {
			// 目标端连接
			bridge.SetTargetConnection(tunnelConn)
			utils.Infof("Tunnel[%s]: set targetConn for existing bridge, connID=%s, hasNetConn=%v", req.TunnelID, conn.ID, netConn != nil)
		}

		// ✅ 切换到流模式（通过接口调用，协议无关）
		if conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if streamModeConn, ok := reader.(interface {
				SetStreamMode(streamMode bool)
			}); ok {
				utils.Infof("Tunnel[%s]: switching connection to stream mode (existing bridge), connID=%s", req.TunnelID, conn.ID)
				streamModeConn.SetStreamMode(true)
			}
		}

		// ✅ 判断是否应该保留在 connMap（通过接口判断，协议无关）
		shouldKeep := false
		if conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if keepConn, ok := reader.(interface {
				ShouldKeepInConnMap() bool
			}); ok {
				shouldKeep = keepConn.ShouldKeepInConnMap()
			}
		}

		if !shouldKeep && req.MappingID != "" {
			s.connLock.Lock()
			delete(s.connMap, connPacket.ConnectionID)
			s.connLock.Unlock()
		}

		return fmt.Errorf("tunnel connected to existing bridge, switching to stream mode")
	}

	if s.tunnelRouting != nil {
		routingState, err := s.tunnelRouting.LookupWaitingTunnel(s.Ctx(), req.TunnelID)
		if err == nil {
			return s.handleCrossServerTargetConnection(conn, req, routingState)
		} else if err != ErrTunnelNotFound && err != ErrTunnelExpired {
			utils.Errorf("Tunnel[%s]: failed to lookup routing state: %v", req.TunnelID, err)
		}
	}

	clientConn := s.getControlConnectionByConnID(connPacket.ConnectionID)
	if clientConn == nil {
		// ✅ 尝试通过 clientID 查找控制连接（HTTP 长轮询等协议，隧道连接和控制连接使用不同的 connID）
		conn := s.getConnectionByConnID(connPacket.ConnectionID)
		if conn != nil && conn.Stream != nil {
			// 尝试从 Stream 直接获取 clientID
			// 1. 尝试从 ServerStreamProcessor 获取（直接实现）
			// 2. 尝试从 httppollStreamAdapter 获取（包装了 ServerStreamProcessor）
			var clientID int64
			if streamWithClientID, ok := conn.Stream.(interface {
				GetClientID() int64
			}); ok {
				clientID = streamWithClientID.GetClientID()
				utils.Infof("Tunnel[%s]: got clientID=%d from stream, connID=%s", req.TunnelID, clientID, connPacket.ConnectionID)
			} else {
				type streamProcessorGetter interface {
					GetStreamProcessor() interface {
						GetClientID() int64
						GetConnectionID() string
						GetMappingID() string
					}
				}
				if adapter, ok := conn.Stream.(streamProcessorGetter); ok {
					// 尝试从适配器获取底层的 StreamProcessorAccessor
					streamProc := adapter.GetStreamProcessor()
					if streamProc != nil {
						clientID = streamProc.GetClientID()
						utils.Infof("Tunnel[%s]: got clientID=%d from stream adapter, connID=%s", req.TunnelID, clientID, connPacket.ConnectionID)
					}
				}
			}

			// 如果获取到 clientID，尝试通过 clientID 查找控制连接
			if clientID > 0 {
				clientConn = s.GetControlConnectionByClientID(clientID)
				if clientConn != nil {
					utils.Infof("Tunnel[%s]: found control connection by clientID=%d, controlConnID=%s", req.TunnelID, clientID, clientConn.GetConnID())
				}
			}

			// 如果还是找不到，尝试创建临时控制连接（通过接口判断，协议无关）
			if clientConn == nil {
				reader := conn.Stream.GetReader()
				if tempConn, ok := reader.(interface {
					CanCreateTemporaryControlConn() bool
					GetClientID() int64
				}); ok && tempConn.CanCreateTemporaryControlConn() {
					utils.Infof("Tunnel[%s]: creating temporary control connection, connID=%s", req.TunnelID, connPacket.ConnectionID)
					var remoteAddr net.Addr
					if conn.RawConn != nil {
						remoteAddr = conn.RawConn.RemoteAddr()
					}
					protocol := conn.Protocol
					if protocol == "" {
						protocol = "tcp"
					}
					newConn := NewControlConnection(conn.ID, conn.Stream, remoteAddr, protocol)
					if clientID > 0 {
						newConn.SetClientID(clientID)
						newConn.SetAuthenticated(true)
						utils.Infof("Tunnel[%s]: set clientID=%d for temporary control connection", req.TunnelID, clientID)
					} else {
						// 尝试从 reader 获取 clientID
						tempClientID := tempConn.GetClientID()
						if tempClientID > 0 {
							newConn.SetClientID(tempClientID)
							newConn.SetAuthenticated(true)
							utils.Infof("Tunnel[%s]: set clientID=%d for temporary control connection (from reader)", req.TunnelID, tempClientID)
						}
					}
					clientConn = newConn
				}
			}
		}
		if clientConn == nil {
			utils.Warnf("Tunnel[%s]: control connection not found for connID %s", req.TunnelID, connPacket.ConnectionID)
			if conn != nil {
				s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
					TunnelID: req.TunnelID,
					Success:  false,
					Error:    "connection not found or not authenticated",
				})
			}
			return fmt.Errorf("control connection not found: %s", connPacket.ConnectionID)
		}
	}

	if err := s.tunnelHandler.HandleTunnelOpen(clientConn, req); err != nil {
		utils.Errorf("Tunnel open failed for connection %s: %v", connPacket.ConnectionID, err)
		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    err.Error(),
		})
		return err
	}

	if req.MappingID != "" {
		s.controlConnLock.Lock()
		if _, exists := s.controlConnMap[connPacket.ConnectionID]; exists {
			delete(s.controlConnMap, connPacket.ConnectionID)
			if clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
				if currentControlConn, exists := s.clientIDIndexMap[clientConn.GetClientID()]; exists && currentControlConn.GetConnID() == connPacket.ConnectionID {
					delete(s.clientIDIndexMap, clientConn.GetClientID())
				}
			}
		}
		s.controlConnLock.Unlock()
	}

	s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  true,
	})

	// 如果有 mappingID，设置映射ID（通过接口调用，协议无关）
	if req.MappingID != "" && clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
		if conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if mappingConn, ok := reader.(interface {
				SetMappingID(mappingID string)
			}); ok {
				utils.Infof("Tunnel[%s]: setting mappingID=%s", req.TunnelID, req.MappingID)
				mappingConn.SetMappingID(req.MappingID)
			}
		}
	}

	// ✅ 对于 HTTP 长轮询透传，clientA 和 clientB 都会创建新连接发送 TunnelOpen
	// 不需要查找"已存在的数据推送连接"，直接使用当前 TunnelOpen 连接
	// 通过 clientID 判断是源端（listen client）还是目标端（target client）

	// 通过接口抽象提取连接（不依赖具体协议）
	netConn := s.extractNetConn(conn)

	// 从控制连接获取 clientID（如果可用）
	var connClientIDFromControl int64
	if clientConn != nil && clientConn.IsAuthenticated() {
		connClientIDFromControl = clientConn.GetClientID()
	}

	// 判断是源端还是目标端连接
	var isSourceClient bool
	if s.cloudControl != nil && req.MappingID != "" {
		mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
		if err == nil {
			listenClientID := mapping.ListenClientID
			if listenClientID == 0 {
				listenClientID = mapping.SourceClientID
			}
			// 从连接中获取 clientID（使用 extractClientID 函数，支持多种方式）
			connClientID := extractClientID(conn.Stream, netConn)
			if connClientID == 0 && connClientIDFromControl > 0 {
				// 如果 extractClientID 返回 0，使用从控制连接获取的 clientID
				connClientID = connClientIDFromControl
			}
			isSourceClient = (connClientID == listenClientID)
			utils.Infof("Tunnel[%s]: identified connection type for new bridge - isSourceClient=%v, connClientID=%d, listenClientID=%d, targetClientID=%d, connID=%s",
				req.TunnelID, isSourceClient, connClientID, listenClientID, mapping.TargetClientID, connPacket.ConnectionID)
		}
	}

	// ✅ 根据 isSourceClient 决定是创建源端bridge还是设置目标端连接
	if isSourceClient {
		// 源端连接：创建新的bridge
		var sourceConn net.Conn
		var sourceStream stream.PackageStreamer
		if conn != nil {
			sourceConn = netConn // 可能为 nil（某些协议不支持 net.Conn）
			sourceStream = conn.Stream
			// 如果 net.Conn 为 nil，尝试从 Stream 创建数据转发器（通过接口抽象）
			if netConn == nil && sourceStream != nil {
				reader := sourceStream.GetReader()
				writer := sourceStream.GetWriter()
				if reader == nil || writer == nil {
					// 该协议不支持桥接（如 HTTP 长轮询），数据已通过协议本身传输
					utils.Infof("Tunnel[%s]: connection does not support net.Conn bridge, data forwarding handled by protocol, connID=%s", req.TunnelID, conn.ID)
				}
			}
			utils.Infof("Tunnel[%s]: extracted sourceConn for new bridge, connID=%s, hasNetConn=%v, hasStream=%v, isSourceClient=%v",
				req.TunnelID, conn.ID, netConn != nil, sourceStream != nil, isSourceClient)
		}

		// ✅ 切换到流模式（通过接口调用，协议无关）
		if conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if streamModeConn, ok := reader.(interface {
				SetStreamMode(streamMode bool)
			}); ok {
				utils.Infof("Tunnel[%s]: switching connection to stream mode", req.TunnelID)
				streamModeConn.SetStreamMode(true)
			}
		}

		if err := s.startSourceBridge(req, sourceConn, sourceStream); err != nil {
			utils.Errorf("Tunnel[%s]: failed to start bridge: %v", req.TunnelID, err)
			return err
		}
	} else {
		// 目标端连接：查找已存在的bridge并设置target连接
		s.bridgeLock.RLock()
		bridge, exists := s.tunnelBridges[req.TunnelID]
		s.bridgeLock.RUnlock()

		if !exists {
			utils.Errorf("Tunnel[%s]: target connection received but bridge not found, connID=%s", req.TunnelID, connPacket.ConnectionID)
			return fmt.Errorf("bridge not found for tunnel %s", req.TunnelID)
		}

		// 创建统一接口连接
		clientID := extractClientID(conn.Stream, netConn)
		tunnelConn := CreateTunnelConnection(conn.ID, netConn, conn.Stream, clientID, req.MappingID, req.TunnelID)

		// 设置目标端连接
		bridge.SetTargetConnection(tunnelConn)
		utils.Infof("Tunnel[%s]: set targetConn for new bridge, connID=%s, hasNetConn=%v", req.TunnelID, conn.ID, netConn != nil)

		// ✅ 切换到流模式（通过接口调用，协议无关）
		if conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if streamModeConn, ok := reader.(interface {
				SetStreamMode(streamMode bool)
			}); ok {
				utils.Infof("Tunnel[%s]: switching target connection to stream mode, connID=%s", req.TunnelID, conn.ID)
				streamModeConn.SetStreamMode(true)
			}
		}
	}

	if req.MappingID != "" {
		s.controlConnLock.Lock()
		if controlConn, exists := s.controlConnMap[connPacket.ConnectionID]; exists {
			delete(s.controlConnMap, connPacket.ConnectionID)
			if controlConn.IsAuthenticated() && controlConn.GetClientID() > 0 {
				if currentControlConn, exists := s.clientIDIndexMap[controlConn.GetClientID()]; exists && currentControlConn.GetConnID() == connPacket.ConnectionID {
					delete(s.clientIDIndexMap, controlConn.GetClientID())
				}
			}
		}
		s.controlConnLock.Unlock()

		// ✅ 判断是否应该保留在 connMap（通过接口判断，协议无关）
		shouldKeep := false
		if conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if keepConn, ok := reader.(interface {
				ShouldKeepInConnMap() bool
			}); ok {
				shouldKeep = keepConn.ShouldKeepInConnMap()
			}
		}

		if !shouldKeep && req.MappingID != "" {
			s.connLock.Lock()
			delete(s.connMap, connPacket.ConnectionID)
			s.connLock.Unlock()
		} else if shouldKeep {
			utils.Debugf("Tunnel[%s]: keeping connection %s in connMap", req.TunnelID, connPacket.ConnectionID)
		}
	}

	return fmt.Errorf("tunnel source connected, switching to stream mode")
}

// ============================================================================
// 辅助方法：响应发送
// ============================================================================

// sendHandshakeResponse 发送握手响应
func (s *SessionManager) sendHandshakeResponse(conn ControlConnectionInterface, resp *packet.HandshakeResponse) error {
	// 序列化响应
	respData, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal handshake response: %w", err)
	}

	// 构造响应包
	respPacket := &packet.TransferPacket{
		PacketType: packet.HandshakeResp,
		Payload:    respData,
	}

	// 发送响应（统一通过 StreamProcessor 处理，所有协议都同步处理）
	utils.Infof("Sending handshake response to connection %s, ClientID=%d", conn.GetConnID(), conn.GetClientID())

	stream := conn.GetStream()
	if stream == nil {
		utils.Errorf("Failed to send handshake response: stream is nil for connection %s", conn.GetConnID())
		return fmt.Errorf("stream is nil")
	}

	// 调试：检查是否是 httppollStreamAdapter，如果是，检查其内部的 ServerStreamProcessor
	utils.Infof("sendHandshakeResponse: stream type=%T, connID=%s", stream, conn.GetConnID())
	type streamProcessorGetter interface {
		GetStreamProcessor() interface {
			GetClientID() int64
			GetConnectionID() string
			GetMappingID() string
		}
	}
	if adapter, ok := stream.(streamProcessorGetter); ok {
		sp := adapter.GetStreamProcessor()
		if sp != nil {
			utils.Infof("sendHandshakeResponse: adapter contains streamProcessor type=%T, connID=%s, clientID=%d", sp, conn.GetConnID(), sp.GetClientID())
		}
	}

	if _, err := stream.WritePacket(respPacket, true, 0); err != nil {
		utils.Errorf("Failed to write handshake response to connection %s: %v", conn.GetConnID(), err)
		return err
	}

	utils.Infof("Handshake response written successfully to connection %s", conn.GetConnID())
	return nil
}

// sendTunnelOpenResponse 发送隧道打开响应
func (s *SessionManager) sendTunnelOpenResponse(conn ControlConnectionInterface, resp *packet.TunnelOpenAckResponse) error {
	// 序列化响应
	respData, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel open response: %w", err)
	}

	// 构造响应包
	respPacket := &packet.TransferPacket{
		PacketType: packet.TunnelOpenAck,
		Payload:    respData,
	}

	// 发送响应
	if _, err := conn.GetStream().WritePacket(respPacket, false, 0); err != nil {
		return fmt.Errorf("failed to write tunnel open response: %w", err)
	}

	return nil
}

// sendTunnelOpenResponseDirect 直接发送隧道打开响应（使用types.Connection）
func (s *SessionManager) sendTunnelOpenResponseDirect(conn *types.Connection, resp *packet.TunnelOpenAckResponse) error {
	// 序列化响应
	respData, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel open response: %w", err)
	}

	// 构造响应包
	respPacket := &packet.TransferPacket{
		PacketType: packet.TunnelOpenAck,
		Payload:    respData,
	}

	utils.Infof("Tunnel[%s]: sending TunnelOpenAck, Success=%v, conn.Protocol=%s", resp.TunnelID, resp.Success, conn.Protocol)
	// 发送响应
	if _, err := conn.Stream.WritePacket(respPacket, true, 0); err != nil {
		utils.Errorf("Tunnel[%s]: failed to write tunnel open response: %v", resp.TunnelID, err)
		return fmt.Errorf("failed to write tunnel open response: %w", err)
	}
	utils.Infof("Tunnel[%s]: TunnelOpenAck sent successfully", resp.TunnelID)

	return nil
}

// ToNetConn 统一接口：将适配层连接转换为 net.Conn
type ToNetConn interface {
	ToNetConn() net.Conn
}

// extractNetConn 从types.Connection中提取底层的net.Conn
func (s *SessionManager) extractNetConn(conn *types.Connection) net.Conn {
	if conn.RawConn != nil {
		return conn.RawConn
	}

	if conn.Stream != nil {
		// 使用接口获取 Reader，而不是类型断言
		reader := conn.Stream.GetReader()

		// 优先使用统一接口
		if toNetConn, ok := reader.(ToNetConn); ok {
			return toNetConn.ToNetConn()
		}

		// 回退：直接实现 net.Conn
		if netConn, ok := reader.(net.Conn); ok {
			return netConn
		}
	}
	return nil
}

// notifyTargetClientToOpenTunnel 通知目标客户端建立隧道连接
func (s *SessionManager) notifyTargetClientToOpenTunnel(req *packet.TunnelOpenRequest) {
	// 1. 获取映射配置
	if s.cloudControl == nil {
		utils.Errorf("Tunnel[%s]: CloudControl not configured, cannot notify target client", req.TunnelID)
		return
	}

	// ✅ 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		utils.Errorf("Tunnel[%s]: failed to get mapping %s: %v", req.TunnelID, req.MappingID, err)
		return
	}

	// 2. 找到目标客户端的控制连接（本地或跨服务器）
	utils.Infof("Tunnel[%s]: looking for target client control connection, targetClientID=%d", req.TunnelID, mapping.TargetClientID)
	targetControlConn := s.GetControlConnectionByClientID(mapping.TargetClientID)
	if targetControlConn == nil {
		// ✅ 某些协议可能没有注册为控制连接，尝试通过 connMap 查找
		utils.Infof("Tunnel[%s]: target client %d control connection not found, searching in connMap", req.TunnelID, mapping.TargetClientID)
		allConns := s.ListConnections()
		for _, c := range allConns {
			if c.Stream != nil {
				reader := c.Stream.GetReader()
				if clientIDConn, ok := reader.(interface {
					GetClientID() int64
				}); ok {
					connClientID := clientIDConn.GetClientID()
					if connClientID == mapping.TargetClientID {
						// 找到目标客户端的连接，创建临时控制连接
						utils.Infof("Tunnel[%s]: found target client connection in connMap, creating temporary control connection, connID=%s", req.TunnelID, c.ID)
						var remoteAddr net.Addr
						if c.RawConn != nil {
							remoteAddr = c.RawConn.RemoteAddr()
						}
						protocol := c.Protocol
						tempConn := NewControlConnection(c.ID, c.Stream, remoteAddr, protocol)
						tempConn.SetClientID(mapping.TargetClientID)
						tempConn.SetAuthenticated(true)
						// 注册为控制连接（临时）
						s.RegisterControlConnection(tempConn)
						targetControlConn = tempConn
						utils.Infof("Tunnel[%s]: created and registered temporary control connection for target client %d", req.TunnelID, mapping.TargetClientID)
						break
					}
				}
			}
		}
	}
	if targetControlConn == nil {
		// ✅ 本地未找到，尝试跨服务器转发
		if s.bridgeManager != nil {
			utils.Infof("Tunnel[%s]: target client %d not on this server, broadcasting to other nodes",
				req.TunnelID, mapping.TargetClientID)
			if err := s.bridgeManager.BroadcastTunnelOpen(req, mapping.TargetClientID); err != nil {
				utils.Errorf("Tunnel[%s]: failed to broadcast to other nodes: %v", req.TunnelID, err)
			} else {
				utils.Infof("Tunnel[%s]: broadcasted to other nodes for client %d",
					req.TunnelID, mapping.TargetClientID)
			}
		} else {
			utils.Errorf("Tunnel[%s]: target client %d not connected and BridgeManager not configured",
				req.TunnelID, mapping.TargetClientID)
		}
		return
	}

	// 3. 构造TunnelOpenRequest命令
	cmdBody := map[string]interface{}{
		"tunnel_id":          req.TunnelID,
		"mapping_id":         req.MappingID,
		"secret_key":         mapping.SecretKey,
		"target_host":        mapping.TargetHost,
		"target_port":        mapping.TargetPort,
		"protocol":           string(mapping.Protocol),
		"enable_compression": mapping.Config.EnableCompression,
		"compression_level":  mapping.Config.CompressionLevel,
		"enable_encryption":  mapping.Config.EnableEncryption,
		"encryption_method":  mapping.Config.EncryptionMethod,
		"encryption_key":     mapping.Config.EncryptionKey,
		"bandwidth_limit":    mapping.Config.BandwidthLimit,
	}

	cmdBodyJSON, err := json.Marshal(cmdBody)
	if err != nil {
		utils.Errorf("Tunnel[%s]: failed to marshal command body: %v", req.TunnelID, err)
		return
	}

	// 4. 通过控制连接发送命令
	cmd := &packet.CommandPacket{
		CommandType: packet.TunnelOpenRequestCmd, // 60
		CommandBody: string(cmdBodyJSON),
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	_, err = targetControlConn.Stream.WritePacket(pkt, false, 0)
	if err != nil {
		utils.Errorf("Tunnel[%s]: failed to send tunnel open request to target client %d: %v",
			req.TunnelID, mapping.TargetClientID, err)
		return
	}

	utils.Infof("Tunnel[%s]: sent TunnelOpenRequest to target client %d via control connection",
		req.TunnelID, mapping.TargetClientID)
}

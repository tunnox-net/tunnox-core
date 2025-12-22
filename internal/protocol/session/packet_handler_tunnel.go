package session

import (
	"encoding/json"
	"fmt"
	"net"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

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
			corelog.Errorf("Failed to parse tunnel open request: %v", err)
			s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
				TunnelID: "",
				Success:  false,
				Error:    fmt.Sprintf("invalid tunnel open request format: %v", err),
			})
			return fmt.Errorf("invalid tunnel open request format: %w", err)
		}
	}

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
			}
		}

		// ✅ 判断是源端还是目标端连接，更新对应的连接
		// 通过 cloudControl 获取映射配置，判断 clientID 是源端还是目标端
		netConn = s.extractNetConn(conn)
		var isSourceClient bool
		if s.cloudControl != nil && req.MappingID != "" {
			mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
			if err == nil {
				// 从连接中获取 clientID（使用 extractClientID 函数，支持多种方式）
				connClientID := extractClientID(conn.Stream, netConn)
				// 如果 extractClientID 返回 0，稍后从控制连接获取（clientConn 在后面定义）
				isSourceClient = (connClientID == mapping.ListenClientID)
			}
		}

		// 创建统一接口连接
		clientID := extractClientID(conn.Stream, netConn)
		tunnelConn := CreateTunnelConnection(conn.ID, netConn, conn.Stream, clientID, req.MappingID, req.TunnelID)

		if isSourceClient {
			// 源端重连，更新 sourceConn
			bridge.SetSourceConnection(tunnelConn)
		} else {
			// 目标端连接
			bridge.SetTargetConnection(tunnelConn)
		}

		// ✅ 切换到流模式（通过接口调用，协议无关）
		if conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if streamModeConn, ok := reader.(interface {
				SetStreamMode(streamMode bool)
			}); ok {
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

	// ✅ 检查是否是跨节点的目标端连接
	// 如果在 TunnelRoutingTable 中找到了路由信息，说明 Bridge 在其他节点
	if s.tunnelRouting != nil {
		_, err := s.tunnelRouting.LookupWaitingTunnel(s.Ctx(), req.TunnelID)
		if err == nil {
			// 提取 net.Conn 用于跨节点转发
			netConn := s.extractNetConn(conn)
			return s.handleCrossNodeTargetConnection(req, conn, netConn)
		} else if err != ErrTunnelNotFound && err != ErrTunnelExpired {
			corelog.Errorf("Tunnel[%s]: failed to lookup routing state: %v", req.TunnelID, err)
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
					}
				}
			}

			// 如果获取到 clientID，尝试通过 clientID 查找控制连接
			if clientID > 0 {
				clientConn = s.GetControlConnectionByClientID(clientID)
			}

			// 如果还是找不到，尝试创建临时控制连接（通过接口判断，协议无关）
			if clientConn == nil {
				reader := conn.Stream.GetReader()
				if tempConn, ok := reader.(interface {
					CanCreateTemporaryControlConn() bool
					GetClientID() int64
				}); ok && tempConn.CanCreateTemporaryControlConn() {
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
					} else {
						// 尝试从 reader 获取 clientID
						tempClientID := tempConn.GetClientID()
						if tempClientID > 0 {
							newConn.SetClientID(tempClientID)
							newConn.SetAuthenticated(true)
						}
					}
					clientConn = newConn
				}
			}
		}
		if clientConn == nil {
			corelog.Warnf("Tunnel[%s]: control connection not found for connID %s", req.TunnelID, connPacket.ConnectionID)
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
		corelog.Errorf("Tunnel open failed for connection %s: %v", connPacket.ConnectionID, err)
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
			// 从连接中获取 clientID（使用 extractClientID 函数，支持多种方式）
			connClientID := extractClientID(conn.Stream, netConn)
			if connClientID == 0 && connClientIDFromControl > 0 {
				// 如果 extractClientID 返回 0，使用从控制连接获取的 clientID
				connClientID = connClientIDFromControl
			}
			isSourceClient = (connClientID == mapping.ListenClientID)
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
				}
			}
		}

		// ✅ 切换到流模式（通过接口调用，协议无关）
		if conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if streamModeConn, ok := reader.(interface {
				SetStreamMode(streamMode bool)
			}); ok {
				streamModeConn.SetStreamMode(true)
			}
		}

		if err := s.startSourceBridge(req, sourceConn, sourceStream); err != nil {
			corelog.Errorf("Tunnel[%s]: failed to start bridge: %v", req.TunnelID, err)
			return err
		}
	} else {
		// 目标端连接：查找已存在的bridge并设置target连接
		s.bridgeLock.RLock()
		bridge, exists := s.tunnelBridges[req.TunnelID]
		s.bridgeLock.RUnlock()

		if !exists {
			// 本地未找到 Bridge，尝试跨节点转发
			if err := s.handleCrossNodeTargetConnection(req, conn, netConn); err != nil {
				corelog.Errorf("Tunnel[%s]: cross-node forwarding failed: %v", req.TunnelID, err)
				return fmt.Errorf("bridge not found for tunnel %s: %w", req.TunnelID, err)
			}
			return nil
		}

		// 创建统一接口连接
		clientID := extractClientID(conn.Stream, netConn)
		tunnelConn := CreateTunnelConnection(conn.ID, netConn, conn.Stream, clientID, req.MappingID, req.TunnelID)

		// 设置目标端连接
		bridge.SetTargetConnection(tunnelConn)

		// ✅ 切换到流模式（通过接口调用，协议无关）
		if conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if streamModeConn, ok := reader.(interface {
				SetStreamMode(streamMode bool)
			}); ok {
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
		}
	}

	return fmt.Errorf("tunnel source connected, switching to stream mode")
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

	// 发送响应
	if _, err := conn.Stream.WritePacket(respPacket, true, 0); err != nil {
		return fmt.Errorf("failed to write tunnel open response: %w", err)
	}

	return nil
}

// notifyTargetClientToOpenTunnel 通知目标客户端建立隧道连接
func (s *SessionManager) notifyTargetClientToOpenTunnel(req *packet.TunnelOpenRequest) {
	// 1. 获取映射配置
	if s.cloudControl == nil {
		corelog.Errorf("Tunnel[%s]: CloudControl not configured, cannot notify target client", req.TunnelID)
		return
	}

	// ✅ 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		corelog.Errorf("Tunnel[%s]: failed to get mapping %s: %v", req.TunnelID, req.MappingID, err)
		return
	}

	// 2. 找到目标客户端的控制连接（本地或跨服务器）
	targetControlConn := s.GetControlConnectionByClientID(mapping.TargetClientID)
	if targetControlConn == nil {
		// 某些协议可能没有注册为控制连接，尝试通过 connMap 查找
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
						break
					}
				}
			}
		}
	}
	if targetControlConn == nil {
		// 本地未找到，尝试跨服务器转发
		if s.bridgeManager != nil {
			s.bridgeManager.BroadcastTunnelOpen(req, mapping.TargetClientID)
		}
		return
	}

	// 3. 构造TunnelOpenRequest命令
	// 对于 SOCKS5 协议，使用请求中的动态目标地址
	// 对于其他协议，使用映射配置中的固定目标地址
	targetHost := mapping.TargetHost
	targetPort := mapping.TargetPort
	if mapping.Protocol == "socks5" && req.TargetHost != "" {
		targetHost = req.TargetHost
		targetPort = req.TargetPort
	}

	cmdBody := map[string]interface{}{
		"tunnel_id":          req.TunnelID,
		"mapping_id":         req.MappingID,
		"secret_key":         mapping.SecretKey,
		"target_host":        targetHost,
		"target_port":        targetPort,
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
		corelog.Errorf("Tunnel[%s]: failed to marshal command body: %v", req.TunnelID, err)
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
		corelog.Errorf("Tunnel[%s]: failed to send tunnel open request to target client %d: %v",
			req.TunnelID, mapping.TargetClientID, err)
	}
}

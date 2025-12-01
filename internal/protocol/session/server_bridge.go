package session

import (
	"fmt"
	"net"
	"time"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// startSourceBridge 创建源端隧道桥接器（用于客户端或服务器侧的源连接）
func (s *SessionManager) startSourceBridge(req *packet.TunnelOpenRequest, sourceConn net.Conn, sourceStream stream.PackageStreamer) error {
	utils.Infof("Tunnel[%s]: startSourceBridge called, mappingID=%s", req.TunnelID, req.MappingID)
	
	if s.cloudControl == nil {
		return fmt.Errorf("cloud control not configured")
	}

	// ✅ 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		utils.Errorf("Tunnel[%s]: failed to get mapping: %v", req.TunnelID, err)
		return fmt.Errorf("mapping not found: %w", err)
	}

	bandwidthLimit := mapping.Config.BandwidthLimit

	bridge := NewTunnelBridge(s.Ctx(), &TunnelBridgeConfig{
		TunnelID:       req.TunnelID,
		MappingID:      req.MappingID,
		SourceConn:     sourceConn,
		SourceStream:   sourceStream,
		BandwidthLimit: bandwidthLimit,
		CloudControl:   s.cloudControl,
	})

	s.bridgeLock.Lock()
	if _, exists := s.tunnelBridges[req.TunnelID]; exists {
		s.bridgeLock.Unlock()
		utils.Warnf("Tunnel[%s]: bridge already exists", req.TunnelID)
		return fmt.Errorf("tunnel %s already exists", req.TunnelID)
	}
	s.tunnelBridges[req.TunnelID] = bridge
	s.bridgeLock.Unlock()
	utils.Infof("Tunnel[%s]: bridge created and registered", req.TunnelID)

	// ✅ 注册到路由表（用于跨服务器隧道）
	if s.tunnelRouting != nil {
		// ✅ 统一使用 ListenClientID（向后兼容：如果为 0 则使用 SourceClientID）
		listenClientID := mapping.ListenClientID
		if listenClientID == 0 {
			listenClientID = mapping.SourceClientID
		}

		routingState := &TunnelWaitingState{
			TunnelID:       req.TunnelID,
			MappingID:      req.MappingID,
			SecretKey:      req.SecretKey,
			SourceNodeID:   s.getNodeID(),
			SourceClientID: listenClientID, // ✅ 使用 ListenClientID
			TargetClientID: mapping.TargetClientID,
			TargetHost:     mapping.TargetHost,
			TargetPort:     mapping.TargetPort,
		}

		if err := s.tunnelRouting.RegisterWaitingTunnel(s.Ctx(), routingState); err != nil {
			utils.Warnf("Tunnel[%s]: failed to register routing state: %v", req.TunnelID, err)
			// 不是致命错误，继续处理
		} else {
			utils.Infof("Tunnel[%s]: registered waiting tunnel (source_node=%s, target_client=%d, ttl=30s)", 
				req.TunnelID, routingState.SourceNodeID, routingState.TargetClientID)
		}
	}

	// 通知目标端客户端建立隧道
	utils.Infof("Tunnel[%s]: notifying target client to open tunnel, targetClientID=%d", req.TunnelID, mapping.TargetClientID)
	go s.notifyTargetClientToOpenTunnel(req)

	// 启动桥接生命周期
	utils.Infof("Tunnel[%s]: starting bridge lifecycle", req.TunnelID)
	go s.runBridgeLifecycle(req.TunnelID, bridge)

	return nil
}

// StartServerTunnel 供服务器内部组件（如 UDP Ingress）使用，创建虚拟源端并发起隧道
func (s *SessionManager) StartServerTunnel(mappingID string, sourceConn net.Conn) (string, error) {
	if s.cloudControl == nil {
		return "", fmt.Errorf("cloud control not configured")
	}

	// ✅ 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		return "", fmt.Errorf("mapping not found: %w", err)
	}

	tunnelID := fmt.Sprintf("server-udp-%s-%d", mappingID, time.Now().UnixNano())
	req := &packet.TunnelOpenRequest{
		MappingID: mappingID,
		TunnelID:  tunnelID,
		SecretKey: mapping.SecretKey,
	}

	if err := s.startSourceBridge(req, sourceConn, nil); err != nil {
		return "", err
	}

	utils.Infof("Server UDP ingress: started tunnel %s for mapping %s", tunnelID, mappingID)
	return tunnelID, nil
}

// runBridgeLifecycle 启动桥接并在结束后清理
func (s *SessionManager) runBridgeLifecycle(tunnelID string, bridge *TunnelBridge) {
	if err := bridge.Start(); err != nil {
		utils.Errorf("Tunnel[%s]: bridge failed: %v", tunnelID, err)
	}

	s.bridgeLock.Lock()
	delete(s.tunnelBridges, tunnelID)
	s.bridgeLock.Unlock()
}

// GetTunnelBridgeByConnectionID 通过 ConnectionID 查找 tunnel bridge
// 统一基于 ConnectionID 寻址
// 返回 interface{} 避免循环依赖
func (s *SessionManager) GetTunnelBridgeByConnectionID(connID string) interface{} {
	if connID == "" {
		return nil
	}

	s.bridgeLock.RLock()
	defer s.bridgeLock.RUnlock()

	// 遍历所有 bridge，检查 sourceConn 或 targetConn 的 ConnectionID
	for tunnelID, bridge := range s.tunnelBridges {
		// 检查 sourceConn 的 ConnectionID
		if bridge.sourceConn != nil {
			if srcConn, ok := bridge.sourceConn.(interface{ GetConnectionID() string }); ok {
				if srcConn.GetConnectionID() == connID {
					utils.Infof("GetTunnelBridgeByConnectionID: found bridge by sourceConn, tunnelID=%s, connID=%s", tunnelID, connID)
					return bridge
				}
			}
		}
		// 检查 targetConn 的 ConnectionID
		if bridge.targetConn != nil {
			if tgtConn, ok := bridge.targetConn.(interface{ GetConnectionID() string }); ok {
				if tgtConn.GetConnectionID() == connID {
					utils.Infof("GetTunnelBridgeByConnectionID: found bridge by targetConn, tunnelID=%s, connID=%s", tunnelID, connID)
					return bridge
				}
			}
		}
	}

	utils.Debugf("GetTunnelBridgeByConnectionID: bridge not found, connID=%s", connID)
	return nil
}

// GetTunnelBridgeByMappingID 通过 mappingID 查找 tunnel bridge（向后兼容）
// 优先使用 GetTunnelBridgeByConnectionID
// 返回 interface{} 避免循环依赖
func (s *SessionManager) GetTunnelBridgeByMappingID(mappingID string, clientID int64) interface{} {
	if mappingID == "" {
		return nil
	}

	s.bridgeLock.RLock()
	defer s.bridgeLock.RUnlock()

	utils.Debugf("GetTunnelBridgeByMappingID: searching for bridge, mappingID=%s, clientID=%d, total bridges=%d",
		mappingID, clientID, len(s.tunnelBridges))

	// 遍历所有 bridge，找到匹配 mappingID 的
	for tunnelID, bridge := range s.tunnelBridges {
		if bridge.mappingID == mappingID {
			// 如果提供了 clientID，进行验证（可选）
			if clientID > 0 && s.cloudControl != nil {
				mapping, err := s.cloudControl.GetPortMapping(mappingID)
				if err == nil {
					listenClientID := mapping.ListenClientID
					if listenClientID == 0 {
						listenClientID = mapping.SourceClientID
					}
					// 验证 clientID 是否匹配
					if clientID != listenClientID && clientID != mapping.TargetClientID {
						continue
					}
				}
			}
			utils.Debugf("GetTunnelBridgeByMappingID: found matching bridge, tunnelID=%s, mappingID=%s", tunnelID, mappingID)
			return bridge
		}
	}

	utils.Debugf("GetTunnelBridgeByMappingID: bridge not found, mappingID=%s, clientID=%d", mappingID, clientID)
	return nil
}

package session

import (
	"fmt"
	"net"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

// startSourceBridge 创建源端隧道桥接器（用于客户端或服务器侧的源连接）
func (s *SessionManager) startSourceBridge(req *packet.TunnelOpenRequest, sourceConn net.Conn, sourceStream stream.PackageStreamer) error {
	corelog.Infof("Tunnel[%s]: startSourceBridge called, mappingID=%s", req.TunnelID, req.MappingID)

	if s.cloudControl == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "cloud control not configured")
	}

	// ✅ 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		corelog.Errorf("Tunnel[%s]: failed to get mapping: %v", req.TunnelID, err)
		return coreerrors.Wrap(err, coreerrors.CodeMappingNotFound, "mapping not found")
	}

	bandwidthLimit := mapping.Config.BandwidthLimit

	// 创建统一接口连接
	var sourceTunnelConn TunnelConnectionInterface
	if sourceConn != nil || sourceStream != nil {
		connID := ""
		if sourceConn != nil {
			connID = sourceConn.RemoteAddr().String()
		}
		clientID := extractClientID(sourceStream, sourceConn)
		sourceTunnelConn = CreateTunnelConnection(connID, sourceConn, sourceStream, clientID, req.MappingID, req.TunnelID)
	}

	bridge := NewTunnelBridge(s.Ctx(), &TunnelBridgeConfig{
		TunnelID:         req.TunnelID,
		MappingID:        req.MappingID,
		SourceTunnelConn: sourceTunnelConn,
		SourceConn:       sourceConn,   // 向后兼容
		SourceStream:     sourceStream, // 向后兼容
		BandwidthLimit:   bandwidthLimit,
		CloudControl:     s.cloudControl,
	})

	s.bridgeLock.Lock()
	if _, exists := s.tunnelBridges[req.TunnelID]; exists {
		s.bridgeLock.Unlock()
		corelog.Warnf("Tunnel[%s]: bridge already exists", req.TunnelID)
		return coreerrors.Newf(coreerrors.CodeAlreadyExists, "tunnel %s already exists", req.TunnelID)
	}
	s.tunnelBridges[req.TunnelID] = bridge
	s.bridgeLock.Unlock()
	corelog.Infof("Tunnel[%s]: bridge created and registered", req.TunnelID)

	// ✅ 注册到路由表（用于跨服务器隧道）
	if s.tunnelRouting != nil {
		routingState := &TunnelWaitingState{
			TunnelID:       req.TunnelID,
			MappingID:      req.MappingID,
			SecretKey:      req.SecretKey,
			SourceNodeID:   s.getNodeID(),
			SourceClientID: mapping.ListenClientID,
			TargetClientID: mapping.TargetClientID,
			TargetHost:     mapping.TargetHost,
			TargetPort:     mapping.TargetPort,
		}

		if err := s.tunnelRouting.RegisterWaitingTunnel(s.Ctx(), routingState); err != nil {
			corelog.Warnf("Tunnel[%s]: failed to register routing state: %v", req.TunnelID, err)
			// 不是致命错误，继续处理
		} else {
			corelog.Infof("Tunnel[%s]: registered waiting tunnel (source_node=%s, target_client=%d, ttl=30s)",
				req.TunnelID, routingState.SourceNodeID, routingState.TargetClientID)

			// ✅ 广播隧道就绪通知，让其他节点知道可以进行跨节点转发
			if s.bridgeManager != nil {
				if err := s.bridgeManager.NotifyTunnelReady(s.Ctx(), req.TunnelID, routingState.SourceNodeID); err != nil {
					corelog.Warnf("Tunnel[%s]: failed to notify tunnel ready: %v", req.TunnelID, err)
				}
			}
		}
	}

	// 通知目标端客户端建立隧道
	corelog.Infof("Tunnel[%s]: notifying target client to open tunnel, targetClientID=%d", req.TunnelID, mapping.TargetClientID)
	go s.notifyTargetClientToOpenTunnel(req)

	// 启动桥接生命周期
	corelog.Infof("Tunnel[%s]: starting bridge lifecycle", req.TunnelID)
	go s.runBridgeLifecycle(req.TunnelID, bridge)

	return nil
}

// StartServerTunnel 供服务器内部组件（如 UDP Ingress）使用，创建虚拟源端并发起隧道
func (s *SessionManager) StartServerTunnel(mappingID string, sourceConn net.Conn) (string, error) {
	if s.cloudControl == nil {
		return "", coreerrors.New(coreerrors.CodeNotConfigured, "cloud control not configured")
	}

	// ✅ 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeMappingNotFound, "mapping not found")
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

	corelog.Infof("Server UDP ingress: started tunnel %s for mapping %s", tunnelID, mappingID)
	return tunnelID, nil
}

// runBridgeLifecycle 启动桥接并在结束后清理
func (s *SessionManager) runBridgeLifecycle(tunnelID string, bridge *TunnelBridge) {
	// 确保 bridge 在函数结束时被关闭，释放所有 goroutines
	defer bridge.Close()

	if err := bridge.Start(); err != nil {
		corelog.Errorf("Tunnel[%s]: bridge failed: %v", tunnelID, err)
	}

	// 清理 bridge
	s.bridgeLock.Lock()
	delete(s.tunnelBridges, tunnelID)
	s.bridgeLock.Unlock()
	corelog.Infof("Tunnel[%s]: bridge removed from map", tunnelID)

	// 清理路由表
	if s.tunnelRouting != nil {
		if err := s.tunnelRouting.RemoveWaitingTunnel(s.Ctx(), tunnelID); err != nil {
			corelog.Warnf("Tunnel[%s]: failed to remove routing state: %v", tunnelID, err)
		} else {
			corelog.Infof("Tunnel[%s]: routing state removed", tunnelID)
		}
	}
}

// GetTunnelBridgeByConnectionID 通过 ConnectionID 查找 tunnel bridge
// 统一基于 ConnectionID 寻址
func (s *SessionManager) GetTunnelBridgeByConnectionID(connID string) TunnelBridgeAccessor {
	if connID == "" {
		return nil
	}

	s.bridgeLock.RLock()
	defer s.bridgeLock.RUnlock()

	// 遍历所有 bridge，检查 sourceConn 或 targetConn 的 ConnectionID
	for tunnelID, bridge := range s.tunnelBridges {
		// 检查 sourceTunnelConn 的 ConnectionID
		if sourceTunnelConn := bridge.GetSourceTunnelConn(); sourceTunnelConn != nil {
			if sourceTunnelConn.GetConnectionID() == connID {
				corelog.Infof("GetTunnelBridgeByConnectionID: found bridge by sourceTunnelConn, tunnelID=%s, connID=%s", tunnelID, connID)
				return bridge
			}
		}
		// 检查 targetTunnelConn 的 ConnectionID
		if targetTunnelConn := bridge.GetTargetTunnelConn(); targetTunnelConn != nil {
			if targetTunnelConn.GetConnectionID() == connID {
				corelog.Infof("GetTunnelBridgeByConnectionID: found bridge by targetTunnelConn, tunnelID=%s, connID=%s", tunnelID, connID)
				return bridge
			}
		}
		// 向后兼容：检查旧接口
		if sourceConn := bridge.GetSourceNetConn(); sourceConn != nil {
			if srcConn, ok := sourceConn.(interface{ GetConnectionID() string }); ok {
				if srcConn.GetConnectionID() == connID {
					corelog.Infof("GetTunnelBridgeByConnectionID: found bridge by sourceConn (legacy), tunnelID=%s, connID=%s", tunnelID, connID)
					return bridge
				}
			}
		}
		if targetConn := bridge.GetTargetNetConn(); targetConn != nil {
			if tgtConn, ok := targetConn.(interface{ GetConnectionID() string }); ok {
				if tgtConn.GetConnectionID() == connID {
					corelog.Infof("GetTunnelBridgeByConnectionID: found bridge by targetConn (legacy), tunnelID=%s, connID=%s", tunnelID, connID)
					return bridge
				}
			}
		}
	}

	corelog.Debugf("GetTunnelBridgeByConnectionID: bridge not found, connID=%s", connID)
	return nil
}

// GetTunnelBridgeByMappingID 通过 mappingID 查找 tunnel bridge（向后兼容）
// 优先使用 GetTunnelBridgeByConnectionID
func (s *SessionManager) GetTunnelBridgeByMappingID(mappingID string, clientID int64) TunnelBridgeAccessor {
	if mappingID == "" {
		return nil
	}

	s.bridgeLock.RLock()
	defer s.bridgeLock.RUnlock()

	corelog.Debugf("GetTunnelBridgeByMappingID: searching for bridge, mappingID=%s, clientID=%d, total bridges=%d",
		mappingID, clientID, len(s.tunnelBridges))

	// 遍历所有 bridge，找到匹配 mappingID 的
	for tunnelID, bridge := range s.tunnelBridges {
		if bridge.GetMappingID() == mappingID {
			// 如果提供了 clientID，进行验证（可选）
			if clientID > 0 && s.cloudControl != nil {
				mapping, err := s.cloudControl.GetPortMapping(mappingID)
				if err == nil {
					// 验证 clientID 是否匹配
					if clientID != mapping.ListenClientID && clientID != mapping.TargetClientID {
						continue
					}
				}
			}
			corelog.Debugf("GetTunnelBridgeByMappingID: found matching bridge, tunnelID=%s, mappingID=%s", tunnelID, mappingID)
			return bridge
		}
	}

	corelog.Debugf("GetTunnelBridgeByMappingID: bridge not found, mappingID=%s, clientID=%d", mappingID, clientID)
	return nil
}

package session

import (
	"fmt"
	"net"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// startSourceBridge 创建源端隧道桥接器（用于客户端或服务器侧的源连接）
func (s *SessionManager) startSourceBridge(req *packet.TunnelOpenRequest, sourceConn net.Conn, sourceStream stream.PackageStreamer) error {
	if s.cloudControl == nil {
		return fmt.Errorf("cloud control not configured")
	}

	mappingInterface, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		return fmt.Errorf("mapping not found: %w", err)
	}

	mapping, ok := mappingInterface.(*models.PortMapping)
	if !ok {
		return fmt.Errorf("invalid mapping type for %s", req.MappingID)
	}

	bandwidthLimit := mapping.Config.BandwidthLimit

	bridge := NewTunnelBridge(&TunnelBridgeConfig{
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
		return fmt.Errorf("tunnel %s already exists", req.TunnelID)
	}
	s.tunnelBridges[req.TunnelID] = bridge
	s.bridgeLock.Unlock()

	// 通知目标端客户端建立隧道
	go s.notifyTargetClientToOpenTunnel(req)

	// 启动桥接生命周期
	go s.runBridgeLifecycle(req.TunnelID, bridge)

	return nil
}

// StartServerTunnel 供服务器内部组件（如 UDP Ingress）使用，创建虚拟源端并发起隧道
func (s *SessionManager) StartServerTunnel(mappingID string, sourceConn net.Conn) (string, error) {
	if s.cloudControl == nil {
		return "", fmt.Errorf("cloud control not configured")
	}

	mappingInterface, err := s.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		return "", fmt.Errorf("mapping not found: %w", err)
	}

	mapping, ok := mappingInterface.(*models.PortMapping)
	if !ok {
		return "", fmt.Errorf("invalid mapping type for %s", mappingID)
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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

// ConfigPusher 配置推送器（负责将映射配置推送给客户端）
type ConfigPusher struct {
	*dispose.ManagerBase
	
	cloudControl   *managers.CloudControl
	sessionManager *session.SessionManager
}

// NewConfigPusher 创建配置推送器
func NewConfigPusher(ctx context.Context, cloudControl *managers.CloudControl, sessionMgr *session.SessionManager) *ConfigPusher {
	return &ConfigPusher{
		ManagerBase:    dispose.NewManager("ConfigPusher", ctx),
		cloudControl:   cloudControl,
		sessionManager: sessionMgr,
	}
}

// PushMappingsToClient 推送映射配置到指定客户端
func (cp *ConfigPusher) PushMappingsToClient(clientID int64) error {
	utils.Infof("ConfigPusher: pushing mappings to client_id=%d", clientID)
	
	// 1. 获取指令连接
	conn := cp.sessionManager.GetControlConnectionByClientID(clientID)
	if conn == nil {
		return fmt.Errorf("client %d not connected", clientID)
	}
	
	// 2. 查询所有映射并过滤该客户端作为源的映射
	allMappings, err := cp.cloudControl.ListPortMappings("")
	if err != nil {
		return fmt.Errorf("failed to list mappings: %w", err)
	}
	
	// 过滤该客户端作为源的映射
	var mappings []*models.PortMapping
	for _, mapping := range allMappings {
		if mapping.SourceClientID == clientID {
			mappings = append(mappings, mapping)
		}
	}
	
	utils.Infof("ConfigPusher: found %d mappings for client_id=%d", len(mappings), clientID)
	
	// 3. 构造配置更新消息
	configUpdate := struct {
		Mappings []MappingConfigDTO `json:"mappings"`
	}{
		Mappings: make([]MappingConfigDTO, 0, len(mappings)),
	}
	
	for _, mapping := range mappings {
		// 只推送 active 状态的映射
		if mapping.Status != models.MappingStatusActive {
			continue
		}
		
		configUpdate.Mappings = append(configUpdate.Mappings, MappingConfigDTO{
			MappingID:  mapping.ID,
			SecretKey:  mapping.SecretKey,
			LocalPort:  mapping.SourcePort,
			TargetHost: mapping.TargetHost,
			TargetPort: mapping.TargetPort,
			
			// ✅ 压缩、加密配置
			EnableCompression: mapping.Config.EnableCompression,
			CompressionLevel:  mapping.Config.CompressionLevel,
			EnableEncryption:  mapping.Config.EnableEncryption,
			EncryptionMethod:  mapping.Config.EncryptionMethod,
			EncryptionKey:     mapping.Config.EncryptionKey,
		})
	}
	
	// 4. 序列化为JSON
	configBody, err := json.Marshal(configUpdate)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	// 5. 构造命令包
	cmdPkt := &packet.CommandPacket{
		CommandType: packet.ConfigGet,
		CommandId:   fmt.Sprintf("config-%d-%d", clientID, time.Now().UnixNano()),
		SenderId:    "server",
		ReceiverId:  fmt.Sprintf("%d", clientID),
		CommandBody: string(configBody),
	}
	
	transferPkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}
	
	// 6. 发送配置
	if _, err := conn.Stream.WritePacket(transferPkt, false, 0); err != nil {
		return fmt.Errorf("failed to send config: %w", err)
	}
	
	utils.Infof("ConfigPusher: config pushed successfully to client_id=%d, %d mappings", clientID, len(configUpdate.Mappings))
	return nil
}

// PushMappingToAllClients 推送单个映射配置到相关客户端
func (cp *ConfigPusher) PushMappingToAllClients(mapping *models.PortMapping) error {
	// 推送给源客户端
	if err := cp.PushMappingsToClient(mapping.SourceClientID); err != nil {
		utils.Warnf("ConfigPusher: failed to push to source client %d: %v", mapping.SourceClientID, err)
	}
	
	return nil
}

// MappingConfigDTO 映射配置DTO（发送给客户端的格式）
type MappingConfigDTO struct {
	MappingID  string `json:"mapping_id"`
	SecretKey  string `json:"secret_key"`
	LocalPort  int    `json:"local_port"`
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
	
	// ✅ 压缩、加密配置
	EnableCompression bool   `json:"enable_compression"`
	CompressionLevel  int    `json:"compression_level"`
	EnableEncryption  bool   `json:"enable_encryption"`
	EncryptionMethod  string `json:"encryption_method"`
	EncryptionKey     string `json:"encryption_key"`
}


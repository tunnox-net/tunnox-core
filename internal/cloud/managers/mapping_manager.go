package managers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/utils"
)

// CreatePortMapping 创建端口映射
func (c *CloudControl) CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error) {
	// 生成端口映射ID，确保不重复
	var mappingID string
	for attempts := 0; attempts < constants.DefaultMaxAttempts; attempts++ {
		generatedID, err := c.idManager.GeneratePortMappingID()
		if err != nil {
			return nil, fmt.Errorf("generate mapping ID failed: %w", err)
		}

		// 检查端口映射是否已存在
		existingMapping, err := c.mappingRepo.GetPortMapping(generatedID)
		if err != nil {
			// 端口映射不存在，可以使用这个ID
			mappingID = generatedID
			break
		}

		if existingMapping != nil {
			// 端口映射已存在，释放ID并重试
			_ = c.idManager.ReleasePortMappingID(generatedID)
			continue
		}

		mappingID = generatedID
		break
	}

	if mappingID == "" {
		return nil, fmt.Errorf("failed to generate unique mapping ID after %d attempts", constants.DefaultMaxAttempts)
	}

	// 生成 SecretKey（用于隧道打开认证）
	secretKey, err := c.idManager.GenerateSecretKey()
	if err != nil {
		_ = c.idManager.ReleasePortMappingID(mappingID)
		return nil, fmt.Errorf("generate secret key failed: %w", err)
	}

	// ✅ 生成加密密钥（如果启用了加密）
	if mapping.Config.EnableEncryption {
		encryptionKey, err := c.generateEncryptionKey(mapping.Config.EncryptionMethod)
		if err != nil {
			_ = c.idManager.ReleasePortMappingID(mappingID)
			return nil, fmt.Errorf("generate encryption key failed: %w", err)
		}
		mapping.Config.EncryptionKey = encryptionKey
		utils.Infof("CloudControl: generated encryption key for mapping %s, method=%s, keyLen=%d",
			mappingID, mapping.Config.EncryptionMethod, len(encryptionKey))
	}

	mapping.ID = mappingID
	mapping.SecretKey = secretKey
	mapping.CreatedAt = time.Now()
	mapping.UpdatedAt = time.Now()

	if err := c.mappingRepo.CreatePortMapping(mapping); err != nil {
		// 如果保存失败，释放ID
		_ = c.idManager.ReleasePortMappingID(mappingID)
		return nil, fmt.Errorf("save port mapping failed: %w", err)
	}

	// 添加到用户的端口映射列表
	if err := c.mappingRepo.AddMappingToUser(mapping.UserID, mapping); err != nil {
		// 如果添加到用户失败，删除端口映射并释放ID
		_ = c.mappingRepo.DeletePortMapping(mappingID)
		_ = c.idManager.ReleasePortMappingID(mappingID)
		return nil, fmt.Errorf("add mapping to user failed: %w", err)
	}

	return mapping, nil
}

// generateEncryptionKey 生成加密密钥（根据加密方法）
func (c *CloudControl) generateEncryptionKey(encryptionMethod string) (string, error) {
	var keySize int

	switch encryptionMethod {
	case "aes-256-gcm":
		keySize = 32 // 256 bits = 32 bytes
	case "aes-128-gcm":
		keySize = 16 // 128 bits = 16 bytes
	case "chacha20-poly1305":
		keySize = 32 // 256 bits = 32 bytes
	default:
		// 默认使用 AES-256-GCM
		keySize = 32
	}

	// 生成随机密钥
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	// 返回hex编码的密钥（方便存储和传输）
	return hex.EncodeToString(key), nil
}

// GetPortMapping 获取端口映射
func (c *CloudControl) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	return c.mappingRepo.GetPortMapping(mappingID)
}

// UpdatePortMapping 更新端口映射
func (c *CloudControl) UpdatePortMapping(mapping *models.PortMapping) error {
	mapping.UpdatedAt = time.Now()
	return c.mappingRepo.UpdatePortMapping(mapping)
}

// DeletePortMapping 删除端口映射
func (c *CloudControl) DeletePortMapping(mappingID string) error {
	// 获取端口映射信息，用于释放ID
	mapping, err := c.mappingRepo.GetPortMapping(mappingID)
	if err == nil && mapping != nil {
		// 释放端口映射ID
		_ = c.idManager.ReleasePortMappingID(mappingID)
	}
	return c.mappingRepo.DeletePortMapping(mappingID)
}

// UpdatePortMappingStatus 更新端口映射状态
func (c *CloudControl) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	return c.mappingRepo.UpdatePortMappingStatus(mappingID, status)
}

// UpdatePortMappingStats 更新端口映射统计
func (c *CloudControl) UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error {
	return c.mappingRepo.UpdatePortMappingStats(mappingID, stats)
}

// ListPortMappings 列出端口映射
func (c *CloudControl) ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error) {
	// 获取所有映射
	mappings, err := c.mappingRepo.ListAllMappings()
	if err != nil {
		return nil, err
	}
	
	// 如果指定了映射类型，进行过滤
	if mappingType == "" {
		return mappings, nil
	}
	
	var filtered []*models.PortMapping
	for _, mapping := range mappings {
		if mapping.Type == mappingType {
			filtered = append(filtered, mapping)
		}
	}
	return filtered, nil
}

// GetUserPortMappings 获取用户的端口映射
func (c *CloudControl) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	return c.mappingRepo.GetUserPortMappings(userID)
}


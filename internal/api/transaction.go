package api

import (
	"fmt"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/utils"
)

// TransactionContext 事务上下文，用于跟踪操作和执行回滚
type TransactionContext struct {
	operations []operation
}

// operation 代表一个可回滚的操作
type operation struct {
	name     string
	rollback func() error
}

// NewTransaction 创建新的事务上下文
func NewTransaction() *TransactionContext {
	return &TransactionContext{
		operations: make([]operation, 0),
	}
}

// AddOperation 添加一个可回滚的操作
func (tx *TransactionContext) AddOperation(name string, rollback func() error) {
	tx.operations = append(tx.operations, operation{
		name:     name,
		rollback: rollback,
	})
}

// Rollback 回滚所有已执行的操作（按相反顺序）
func (tx *TransactionContext) Rollback() error {
	utils.Warnf("Transaction: rolling back %d operations", len(tx.operations))

	var lastErr error
	// 反向执行回滚操作
	for i := len(tx.operations) - 1; i >= 0; i-- {
		op := tx.operations[i]
		utils.Infof("Transaction: rolling back operation '%s'", op.name)

		if err := op.rollback(); err != nil {
			utils.Errorf("Transaction: failed to rollback operation '%s': %v", op.name, err)
			lastErr = err
			// 继续尝试回滚其他操作
		} else {
			utils.Infof("Transaction: successfully rolled back operation '%s'", op.name)
		}
	}

	return lastErr
}

// createMappingWithTransaction 创建映射（带事务）
func (s *ManagementAPIServer) createMappingWithTransaction(mapping *models.PortMapping) (*models.PortMapping, error) {
	tx := NewTransaction()

	// 1. 创建映射
	createdMapping, err := s.cloudControl.CreatePortMapping(mapping)
	if err != nil {
		return nil, fmt.Errorf("failed to create mapping: %w", err)
	}

	// 记录回滚操作：删除映射
	tx.AddOperation("CreatePortMapping", func() error {
		return s.cloudControl.DeletePortMapping(createdMapping.ID)
	})

	// 2. 推送配置给客户端（这个操作是异步的，不需要回滚）
	s.pushMappingToClients(createdMapping)

	// 如果后续有更多操作失败，可以调用 tx.Rollback()
	// 这里示例简单，没有更多操作

	return createdMapping, nil
}

// claimClientWithTransaction 认领客户端（带事务）
func (s *ManagementAPIServer) claimClientWithTransaction(anonClientID int64, userID, newClientName string) (map[string]interface{}, error) {
	tx := NewTransaction()

	// 1. 获取匿名客户端
	anonClient, err := s.cloudControl.GetAnonymousClient(anonClientID)
	if err != nil {
		return nil, fmt.Errorf("anonymous client not found: %w", err)
	}

	// 2. 创建新的注册客户端
	newClient, err := s.cloudControl.CreateClient(userID, newClientName)
	if err != nil {
		return nil, fmt.Errorf("failed to create new client: %w", err)
	}

	// 记录回滚操作：删除新创建的客户端
	tx.AddOperation("CreateClient", func() error {
		return s.cloudControl.DeleteClient(newClient.ID)
	})

	// 3. 生成 JWT token
	tokenInfo, err := s.cloudControl.GenerateJWTToken(newClient.ID)
	if err != nil {
		// 回滚：删除新创建的客户端
		if rbErr := tx.Rollback(); rbErr != nil {
			utils.Errorf("API: failed to rollback after token generation failure: %v", rbErr)
		}
		return nil, fmt.Errorf("failed to generate JWT token: %w", err)
	}

	// 记录回滚操作：撤销 token（实际上token不需要回滚，因为客户端还未获取）

	// 4. 标记匿名客户端为禁用
	originalStatus := anonClient.Status
	anonClient.Status = models.ClientStatusBlocked
	if err := s.cloudControl.UpdateClient(anonClient); err != nil {
		// 回滚之前的操作
		if rbErr := tx.Rollback(); rbErr != nil {
			utils.Errorf("API: failed to rollback after client update failure: %v", rbErr)
		}
		return nil, fmt.Errorf("failed to update anonymous client: %w", err)
	}

	// 记录回滚操作：恢复匿名客户端状态
	tx.AddOperation("UpdateAnonymousClient", func() error {
		anonClient.Status = originalStatus
		return s.cloudControl.UpdateClient(anonClient)
	})

	// 5. 迁移端口映射
	if err := s.cloudControl.MigrateClientMappings(anonClient.ID, newClient.ID); err != nil {
		// 迁移失败只记录警告，不回滚（因为映射可能部分成功）
		utils.Warnf("API: failed to migrate mappings from client %d to %d: %v", anonClient.ID, newClient.ID, err)
	}

	// 成功完成，不需要回滚
	return map[string]interface{}{
		"client_id":  newClient.ID,
		"auth_token": tokenInfo.Token,
		"expires_at": tokenInfo.ExpiresAt,
		"message":    "Client claimed successfully. Please save your credentials.",
	}, nil
}

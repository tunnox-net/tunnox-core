package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 隧道迁移管理
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// MigrationStatus 迁移状态
type MigrationStatus string

const (
	MigrationStatusIdle       MigrationStatus = "idle"        // 空闲
	MigrationStatusInitiated  MigrationStatus = "initiated"   // 已发起
	MigrationStatusInProgress MigrationStatus = "in_progress" // 进行中
	MigrationStatusCompleted  MigrationStatus = "completed"   // 已完成
	MigrationStatusFailed     MigrationStatus = "failed"      // 失败
)

// TunnelMigrationInfo 隧道迁移信息
type TunnelMigrationInfo struct {
	TunnelID       string
	Status         MigrationStatus
	SourceNodeID   string
	TargetNodeID   string
	InitiatedAt    time.Time
	CompletedAt    *time.Time
	Error          string
}

// TunnelMigrationManager 隧道迁移管理器
//
// 职责：
// 1. 发起隧道迁移
// 2. 跟踪迁移状态
// 3. 协调源节点和目标节点
type TunnelMigrationManager struct {
	mu sync.RWMutex

	// 迁移状态跟踪
	migrations map[string]*TunnelMigrationInfo // tunnelID -> migration info

	// 依赖组件
	stateManager *TunnelStateManager
	sessionMgr   *SessionManager

	// 配置
	migrationTimeout time.Duration // 迁移超时（默认30秒）
}

// NewTunnelMigrationManager 创建隧道迁移管理器
func NewTunnelMigrationManager(stateManager *TunnelStateManager, sessionMgr *SessionManager) *TunnelMigrationManager {
	return &TunnelMigrationManager{
		migrations:       make(map[string]*TunnelMigrationInfo),
		stateManager:     stateManager,
		sessionMgr:       sessionMgr,
		migrationTimeout: 30 * time.Second,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 迁移发起（源节点）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// InitiateMigration 发起隧道迁移
//
// 在源节点调用，将隧道迁移到目标节点。
func (m *TunnelMigrationManager) InitiateMigration(
	ctx context.Context,
	tunnelID string,
	targetNodeID string,
	tunnelState *TunnelState,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已经在迁移中
	if info, exists := m.migrations[tunnelID]; exists {
		if info.Status == MigrationStatusInProgress {
			return fmt.Errorf("tunnel already migrating: %s", tunnelID)
		}
	}

	// 创建迁移信息
	migrationInfo := &TunnelMigrationInfo{
		TunnelID:     tunnelID,
		Status:       MigrationStatusInitiated,
		SourceNodeID: m.sessionMgr.GetNodeID(),
		TargetNodeID: targetNodeID,
		InitiatedAt:  time.Now(),
	}
	m.migrations[tunnelID] = migrationInfo

	utils.Infof("TunnelMigration: initiated migration for tunnel %s to node %s",
		tunnelID, targetNodeID)

	// 保存状态到存储（供目标节点读取）
	if err := m.stateManager.SaveState(tunnelState); err != nil {
		migrationInfo.Status = MigrationStatusFailed
		migrationInfo.Error = err.Error()
		return fmt.Errorf("failed to save tunnel state: %w", err)
	}

	// 标记为进行中
	migrationInfo.Status = MigrationStatusInProgress

	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 迁移接受（目标节点）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// AcceptMigration 接受隧道迁移
//
// 在目标节点调用，从存储中加载隧道状态并恢复。
func (m *TunnelMigrationManager) AcceptMigration(
	ctx context.Context,
	cmd *packet.TunnelMigrateCommand,
) (*TunnelState, error) {
	utils.Infof("TunnelMigration: accepting migration for tunnel %s from node %s",
		cmd.TunnelID, cmd.SourceNodeID)

	// 从存储加载状态
	state, err := m.stateManager.LoadState(cmd.TunnelID)
	if err != nil {
		return nil, fmt.Errorf("failed to load tunnel state: %w", err)
	}

	// 验证状态签名
	if state.Signature != cmd.StateSignature {
		return nil, errors.New("state signature mismatch")
	}

	// 记录迁移信息
	m.mu.Lock()
	migrationInfo := &TunnelMigrationInfo{
		TunnelID:     cmd.TunnelID,
		Status:       MigrationStatusInProgress,
		SourceNodeID: cmd.SourceNodeID,
		TargetNodeID: cmd.TargetNodeID,
		InitiatedAt:  time.Now(),
	}
	m.migrations[cmd.TunnelID] = migrationInfo
	m.mu.Unlock()

	utils.Infof("TunnelMigration: successfully loaded state for tunnel %s", cmd.TunnelID)

	return state, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 迁移完成
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CompleteMigration 完成隧道迁移
//
// 标记迁移完成，清理状态。
func (m *TunnelMigrationManager) CompleteMigration(tunnelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.migrations[tunnelID]
	if !exists {
		return fmt.Errorf("migration info not found for tunnel %s", tunnelID)
	}

	now := time.Now()
	info.Status = MigrationStatusCompleted
	info.CompletedAt = &now

	utils.Infof("TunnelMigration: completed migration for tunnel %s (duration: %v)",
		tunnelID, now.Sub(info.InitiatedAt))

	// 删除存储中的状态（已恢复，不再需要）
	if err := m.stateManager.DeleteState(tunnelID); err != nil {
		utils.Warnf("TunnelMigration: failed to delete state for tunnel %s: %v", tunnelID, err)
	}

	return nil
}

// FailMigration 标记迁移失败
func (m *TunnelMigrationManager) FailMigration(tunnelID string, reason error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.migrations[tunnelID]
	if !exists {
		return
	}

	info.Status = MigrationStatusFailed
	info.Error = reason.Error()

	utils.Warnf("TunnelMigration: migration failed for tunnel %s: %v", tunnelID, reason)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 状态查询
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetMigrationInfo 获取迁移信息
func (m *TunnelMigrationManager) GetMigrationInfo(tunnelID string) (*TunnelMigrationInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.migrations[tunnelID]
	if !exists {
		return nil, fmt.Errorf("migration info not found for tunnel %s", tunnelID)
	}

	return info, nil
}

// IsMigrating 检查隧道是否正在迁移
func (m *TunnelMigrationManager) IsMigrating(tunnelID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.migrations[tunnelID]
	if !exists {
		return false
	}

	return info.Status == MigrationStatusInProgress
}

// GetActiveMigrations 获取所有活跃的迁移
func (m *TunnelMigrationManager) GetActiveMigrations() []*TunnelMigrationInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]*TunnelMigrationInfo, 0)
	for _, info := range m.migrations {
		if info.Status == MigrationStatusInProgress {
			active = append(active, info)
		}
	}

	return active
}

// CleanupOldMigrations 清理旧的迁移记录
//
// 清理超过1小时的已完成或失败的迁移记录。
func (m *TunnelMigrationManager) CleanupOldMigrations() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)

	for tunnelID, info := range m.migrations {
		if info.Status == MigrationStatusCompleted || info.Status == MigrationStatusFailed {
			if info.InitiatedAt.Before(cutoff) {
				delete(m.migrations, tunnelID)
			}
		}
	}
}


package session

import (
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/utils"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Phase 2 集成: 隧道迁移支持
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SetTunnelStateManager 设置隧道状态管理器
func (s *SessionManager) SetTunnelStateManager(mgr *TunnelStateManager) {
	s.tunnelStateManager = mgr
	utils.Debug("TunnelStateManager configured in SessionManager")
}

// SetMigrationManager 设置迁移管理器
func (s *SessionManager) SetMigrationManager(mgr *TunnelMigrationManager) {
	s.migrationManager = mgr
	utils.Debug("MigrationManager configured in SessionManager")
}

// SaveActiveTunnelStates 保存所有活跃隧道状态（在服务器关闭前调用）
//
// 这个方法会：
// 1. 遍历所有活跃的隧道连接
// 2. 为每个隧道创建状态快照
// 3. 保存到Redis（通过TunnelStateManager）
func (s *SessionManager) SaveActiveTunnelStates() error {
	if s.tunnelStateManager == nil {
		return fmt.Errorf("tunnel state manager not configured")
	}

	s.tunnelConnLock.RLock()
	tunnels := make([]*TunnelConnection, 0, len(s.tunnelConnMap))
	for _, conn := range s.tunnelConnMap {
		tunnels = append(tunnels, conn)
	}
	s.tunnelConnLock.RUnlock()

	utils.Infof("SessionManager: saving states for %d active tunnels", len(tunnels))

	savedCount := 0
	failedCount := 0

	for _, tunnel := range tunnels {
		// 创建隧道状态快照
		state := &TunnelState{
			TunnelID:   tunnel.TunnelID,
			MappingID:  tunnel.MappingID,
			CreatedAt:  tunnel.CreatedAt,
			UpdatedAt:  tunnel.LastActiveAt,
		}

		// 如果隧道启用了序列号，保存缓冲区状态
		if tunnel.enableSeqNum {
			state.LastSeqNum = tunnel.sendBuffer.GetNextSeq() - 1
			state.LastAckNum = tunnel.sendBuffer.GetConfirmedSeq()
			state.NextExpectedSeq = tunnel.receiveBuffer.GetNextExpected()
			state.BufferedPackets = CaptureSendBufferState(tunnel.sendBuffer)
		}

		// 保存状态
		if err := s.tunnelStateManager.SaveState(state); err != nil {
			utils.Warnf("SessionManager: failed to save state for tunnel %s: %v", tunnel.TunnelID, err)
			failedCount++
			continue
		}

		savedCount++
	}

	utils.Infof("SessionManager: saved tunnel states (success=%d, failed=%d)", savedCount, failedCount)

	if failedCount > 0 {
		return fmt.Errorf("failed to save %d tunnel states", failedCount)
	}

	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 隧道恢复Token
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// TunnelResumeToken 隧道恢复Token
//
// 用于客户端重连后恢复隧道，包含：
// - TunnelID: 隧道ID
// - Signature: 状态签名（验证完整性）
type TunnelResumeToken struct {
	TunnelID  string `json:"tunnel_id"`
	Signature string `json:"signature"`
	IssuedAt  int64  `json:"issued_at"` // Unix timestamp
}

// GenerateTunnelResumeToken 生成隧道恢复Token
//
// 客户端在收到ServerShutdown命令时，可以为活跃隧道生成恢复Token，
// 用于后续重连恢复。
func (s *SessionManager) GenerateTunnelResumeToken(tunnelID string) (string, error) {
	if s.tunnelStateManager == nil {
		return "", fmt.Errorf("tunnel state manager not configured")
	}

	// 从存储加载隧道状态
	state, err := s.tunnelStateManager.LoadState(tunnelID)
	if err != nil {
		return "", fmt.Errorf("failed to load tunnel state: %w", err)
	}

	// 创建恢复Token
	resumeToken := &TunnelResumeToken{
		TunnelID:  tunnelID,
		Signature: state.Signature,
		IssuedAt:  time.Now().Unix(),
	}

	// 序列化为JSON
	tokenJSON, err := json.Marshal(resumeToken)
	if err != nil {
		return "", fmt.Errorf("failed to marshal resume token: %w", err)
	}

	return string(tokenJSON), nil
}

// ValidateTunnelResumeToken 验证隧道恢复Token
//
// 在HandleTunnelOpen中调用，验证Token并返回隧道状态。
func (s *SessionManager) ValidateTunnelResumeToken(tokenStr string) (*TunnelState, error) {
	if s.tunnelStateManager == nil {
		return nil, fmt.Errorf("tunnel state manager not configured")
	}

	// 解析Token
	var token TunnelResumeToken
	if err := json.Unmarshal([]byte(tokenStr), &token); err != nil {
		return nil, fmt.Errorf("invalid resume token format: %w", err)
	}

	// 从存储加载隧道状态
	state, err := s.tunnelStateManager.LoadState(token.TunnelID)
	if err != nil {
		return nil, fmt.Errorf("failed to load tunnel state: %w", err)
	}

	// 验证签名
	if state.Signature != token.Signature {
		return nil, fmt.Errorf("resume token signature mismatch")
	}

	// 检查Token是否过期（默认5分钟）
	issuedAt := time.Unix(token.IssuedAt, 0)
	if time.Since(issuedAt) > 5*time.Minute {
		return nil, fmt.Errorf("resume token expired (issued at %s)", issuedAt.Format(time.RFC3339))
	}

	utils.Infof("SessionManager: validated resume token for tunnel %s", token.TunnelID)
	return state, nil
}

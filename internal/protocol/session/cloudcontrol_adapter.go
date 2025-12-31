package session

import (
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/protocol/session/core"
)

// ============================================================================
// 向后兼容别名 - CloudControlAdapter 已迁移到 core 子包
// ============================================================================

// CloudControlAdapter 适配器别名（向后兼容）
// Deprecated: 请使用 core.CloudControlAdapter
type CloudControlAdapter = core.CloudControlAdapter

// NewCloudControlAdapter 创建适配器（向后兼容）
// Deprecated: 请使用 core.NewCloudControlAdapter
func NewCloudControlAdapter(cc *managers.BuiltinCloudControl) CloudControlAPI {
	return core.NewCloudControlAdapter(cc)
}

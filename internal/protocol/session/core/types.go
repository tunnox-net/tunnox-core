package core

// ShutdownReason 关闭原因
type ShutdownReason string

const (
	ShutdownReasonRollingUpdate ShutdownReason = "rolling_update" // 滚动更新
	ShutdownReasonMaintenance   ShutdownReason = "maintenance"    // 维护
	ShutdownReasonShutdown      ShutdownReason = "shutdown"       // 正常关闭
)

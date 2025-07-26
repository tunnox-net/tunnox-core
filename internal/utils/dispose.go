package utils

import (
	"tunnox-core/internal/core/dispose"
)

// 类型别名，保持向后兼容
type Dispose = dispose.Dispose
type DisposeError = dispose.DisposeError
type DisposeResult = dispose.DisposeResult
type Disposable = dispose.Disposable

// 全局函数别名，保持向后兼容
var (
	NewResourceManager                   = dispose.NewResourceManager
	RegisterGlobalResource               = dispose.RegisterGlobalResource
	UnregisterGlobalResource             = dispose.UnregisterGlobalResource
	DisposeAllGlobalResources            = dispose.DisposeAllGlobalResources
	DisposeAllGlobalResourcesWithTimeout = dispose.DisposeAllGlobalResourcesWithTimeout
)

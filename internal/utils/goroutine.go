package utils

import (
	"runtime/debug"
	corelog "tunnox-core/internal/core/log"
)

// SafeGo 安全地启动一个 goroutine，捕获并记录 panic
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				corelog.Errorf("FATAL: goroutine '%s' panic recovered: %v", name, r)
				corelog.Errorf("Stack trace:\n%s", string(debug.Stack()))
			}
		}()
		fn()
	}()
}

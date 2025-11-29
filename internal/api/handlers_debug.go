package api

import (
	"bytes"
	"fmt"
	"net/http"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"
)

// GoroutineInfo goroutine 信息
type GoroutineInfo struct {
	ID         int    `json:"id"`
	State      string `json:"state"`
	Stack      string `json:"stack,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	WaitReason string `json:"wait_reason,omitempty"`
}

// GoroutineStats goroutine 统计信息
type GoroutineStats struct {
	Total        int            `json:"total"`
	ByState      map[string]int `json:"by_state"`
	Goroutines   []GoroutineInfo `json:"goroutines,omitempty"`
	StackSummary string         `json:"stack_summary,omitempty"`
}

// handleGetGoroutines 获取所有 goroutine 信息
func (s *ManagementAPIServer) handleGetGoroutines(w http.ResponseWriter, r *http.Request) {
	// 解析查询参数
	includeStack := r.URL.Query().Get("stack") == "true"
	includeSummary := r.URL.Query().Get("summary") == "true"
	limitStr := r.URL.Query().Get("limit")
	limit := 0
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 0 {
			s.respondError(w, http.StatusBadRequest, "Invalid limit parameter")
			return
		}
	}

	// 获取 goroutine 数量
	numGoroutines := runtime.NumGoroutine()

	// 获取所有 goroutine 的堆栈
	buf := make([]byte, 1024*1024) // 1MB buffer
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}

	// 解析堆栈信息
	goroutines := parseGoroutineStacks(buf, includeStack)
	stats := GoroutineStats{
		Total:      numGoroutines,
		ByState:    make(map[string]int),
		Goroutines: goroutines,
	}

	// 统计状态
	for _, g := range goroutines {
		stats.ByState[g.State]++
	}

	// 限制返回数量
	if limit > 0 && len(stats.Goroutines) > limit {
		stats.Goroutines = stats.Goroutines[:limit]
	}

	// 生成堆栈摘要
	if includeSummary {
		stats.StackSummary = generateStackSummary(buf)
	}

	s.respondJSON(w, http.StatusOK, stats)
}

// handleGetGoroutineProfile 获取 goroutine profile (pprof 格式)
func (s *ManagementAPIServer) handleGetGoroutineProfile(w http.ResponseWriter, r *http.Request) {
	// 获取 pprof 参数
	debug := r.URL.Query().Get("debug")
	debugLevel := 0
	if debug != "" {
		var err error
		debugLevel, err = strconv.Atoi(debug)
		if err != nil || debugLevel < 0 || debugLevel > 2 {
			debugLevel = 0
		}
	}

	// 获取 goroutine profile
	profile := pprof.Lookup("goroutine")
	if profile == nil {
		s.respondError(w, http.StatusInternalServerError, "Failed to get goroutine profile")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if debugLevel > 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		err := profile.WriteTo(w, debugLevel)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write profile: %v", err))
			return
		}
	} else {
		// 使用 JSON 格式返回
		var buf bytes.Buffer
		err := profile.WriteTo(&buf, 0)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write profile: %v", err))
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]string{
			"profile": buf.String(),
		})
	}
}

// handleGetGoroutineCount 获取 goroutine 数量统计
func (s *ManagementAPIServer) handleGetGoroutineCount(w http.ResponseWriter, r *http.Request) {
	numGoroutines := runtime.NumGoroutine()

	// 获取堆栈信息用于分析
	buf := make([]byte, 1024*1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}

	goroutines := parseGoroutineStacks(buf, false)
	byState := make(map[string]int)
	for _, g := range goroutines {
		byState[g.State]++
	}

	stats := map[string]interface{}{
		"total":    numGoroutines,
		"by_state": byState,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	s.respondJSON(w, http.StatusOK, stats)
}

// parseGoroutineStacks 解析 goroutine 堆栈信息
func parseGoroutineStacks(stack []byte, includeStack bool) []GoroutineInfo {
	var goroutines []GoroutineInfo
	lines := strings.Split(string(stack), "\n")

	var currentGoroutine *GoroutineInfo
	var stackLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 检查是否是 goroutine 头部行
		if strings.HasPrefix(line, "goroutine ") {
			// 保存上一个 goroutine
			if currentGoroutine != nil {
				if includeStack && len(stackLines) > 0 {
					currentGoroutine.Stack = strings.Join(stackLines, "\n")
				}
				goroutines = append(goroutines, *currentGoroutine)
			}

		// 解析新的 goroutine 信息
		// 格式: "goroutine 1 [running]:"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			currentGoroutine = &GoroutineInfo{}
			// 提取 ID (格式: "goroutine 123 [state]")
			if idStr := strings.TrimPrefix(parts[0], "goroutine"); idStr != parts[0] {
				if len(parts) > 1 {
					if id, err := strconv.Atoi(parts[1]); err == nil {
						currentGoroutine.ID = id
					}
				}
			}
			// 提取状态 (格式: "[running]" 或 "[chan receive, 123 minutes]")
			if len(parts) >= 3 {
				statePart := parts[2]
				// 移除方括号和冒号
				statePart = strings.Trim(statePart, "[]:")
				// 提取状态（可能包含逗号分隔的额外信息）
				if commaIdx := strings.Index(statePart, ","); commaIdx > 0 {
					statePart = statePart[:commaIdx]
				}
				currentGoroutine.State = strings.TrimSpace(statePart)
			}
			stackLines = []string{}
		}
		} else if currentGoroutine != nil {
			// 堆栈行
			if includeStack {
				stackLines = append(stackLines, line)
			}
			// 尝试提取等待原因
			if strings.Contains(line, "chan receive") || strings.Contains(line, "chan send") {
				currentGoroutine.WaitReason = extractWaitReason(line)
			}
		}
	}

	// 保存最后一个 goroutine
	if currentGoroutine != nil {
		if includeStack && len(stackLines) > 0 {
			currentGoroutine.Stack = strings.Join(stackLines, "\n")
		}
		goroutines = append(goroutines, *currentGoroutine)
	}

	return goroutines
}

// extractWaitReason 提取等待原因
func extractWaitReason(line string) string {
	if strings.Contains(line, "chan receive") {
		return "channel receive"
	}
	if strings.Contains(line, "chan send") {
		return "channel send"
	}
	if strings.Contains(line, "select") {
		return "select"
	}
	if strings.Contains(line, "sleep") {
		return "sleep"
	}
	if strings.Contains(line, "syscall") {
		return "syscall"
	}
	return "unknown"
}

// generateStackSummary 生成堆栈摘要
func generateStackSummary(stack []byte) string {
	lines := strings.Split(string(stack), "\n")
	summary := make(map[string]int)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 提取函数调用
		if strings.HasPrefix(line, "\t") && strings.Contains(line, "(") {
			// 提取包名和函数名
			parts := strings.Split(line, "(")
			if len(parts) > 0 {
				funcName := strings.TrimSpace(parts[0])
				// 提取最后的函数名
				if idx := strings.LastIndex(funcName, "."); idx >= 0 {
					funcName = funcName[idx+1:]
				}
				summary[funcName]++
			}
		}
	}

	var result strings.Builder
	result.WriteString("Top functions:\n")
	for funcName, count := range summary {
		if count > 1 {
			result.WriteString(fmt.Sprintf("  %s: %d\n", funcName, count))
		}
	}

	return result.String()
}

// MemoryStats 内存统计信息
type MemoryStats struct {
	Alloc         uint64            `json:"alloc"`          // 当前分配的字节数
	TotalAlloc    uint64            `json:"total_alloc"`    // 累计分配的字节数
	Sys           uint64            `json:"sys"`            // 从系统获取的字节数
	Lookups       uint64            `json:"lookups"`        // 指针查找次数
	Mallocs       uint64            `json:"mallocs"`        // 分配次数
	Frees         uint64            `json:"frees"`          // 释放次数
	HeapAlloc     uint64            `json:"heap_alloc"`     // 堆分配的字节数
	HeapSys       uint64            `json:"heap_sys"`       // 堆从系统获取的字节数
	HeapIdle      uint64            `json:"heap_idle"`      // 堆空闲字节数
	HeapInuse     uint64            `json:"heap_inuse"`     // 堆使用中的字节数
	HeapReleased  uint64            `json:"heap_released"`  // 释放回系统的字节数
	HeapObjects   uint64            `json:"heap_objects"`   // 堆对象数量
	StackInuse    uint64            `json:"stack_inuse"`    // 栈使用中的字节数
	StackSys      uint64            `json:"stack_sys"`      // 栈从系统获取的字节数
	MSpanInuse    uint64            `json:"mspan_inuse"`    // mspan 使用中的字节数
	MSpanSys      uint64            `json:"mspan_sys"`      // mspan 从系统获取的字节数
	MCacheInuse   uint64            `json:"mcache_inuse"`   // mcache 使用中的字节数
	MCacheSys     uint64            `json:"mcache_sys"`     // mcache 从系统获取的字节数
	BuckHashSys   uint64            `json:"buck_hash_sys"`  // bucket hash 表从系统获取的字节数
	GCSys         uint64            `json:"gc_sys"`         // GC 元数据从系统获取的字节数
	OtherSys      uint64            `json:"other_sys"`       // 其他系统分配的字节数
	NextGC        uint64            `json:"next_gc"`        // 下次 GC 的目标堆大小
	LastGC        uint64            `json:"last_gc"`         // 上次 GC 的时间（纳秒）
	PauseTotalNs  uint64            `json:"pause_total_ns"` // GC 暂停总时间（纳秒）
	NumGC         uint32            `json:"num_gc"`         // GC 次数
	NumForcedGC   uint32            `json:"num_forced_gc"`  // 强制 GC 次数
	GCCPUFraction float64           `json:"gc_cpu_fraction"` // GC CPU 使用率
	EnableGC      bool              `json:"enable_gc"`      // GC 是否启用
	DebugGC       bool              `json:"debug_gc"`       // GC 调试模式
	BySize        []SizeClassStats  `json:"by_size,omitempty"` // 按大小分类的统计
	Timestamp     string            `json:"timestamp"`      // 时间戳
}

// SizeClassStats 大小分类统计
type SizeClassStats struct {
	Size     uint32 `json:"size"`     // 对象大小
	Mallocs  uint64 `json:"mallocs"`  // 分配次数
	Frees    uint64 `json:"frees"`    // 释放次数
}

// handleGetMemoryStats 获取内存统计信息
func (s *ManagementAPIServer) handleGetMemoryStats(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := MemoryStats{
		Alloc:         m.Alloc,
		TotalAlloc:    m.TotalAlloc,
		Sys:           m.Sys,
		Lookups:       m.Lookups,
		Mallocs:       m.Mallocs,
		Frees:         m.Frees,
		HeapAlloc:     m.HeapAlloc,
		HeapSys:       m.HeapSys,
		HeapIdle:      m.HeapIdle,
		HeapInuse:     m.HeapInuse,
		HeapReleased:  m.HeapReleased,
		HeapObjects:   m.HeapObjects,
		StackInuse:    m.StackInuse,
		StackSys:      m.StackSys,
		MSpanInuse:    m.MSpanInuse,
		MSpanSys:      m.MSpanSys,
		MCacheInuse:   m.MCacheInuse,
		MCacheSys:     m.MCacheSys,
		BuckHashSys:   m.BuckHashSys,
		GCSys:         m.GCSys,
		OtherSys:      m.OtherSys,
		NextGC:        m.NextGC,
		LastGC:        m.LastGC,
		PauseTotalNs:  m.PauseTotalNs,
		NumGC:         m.NumGC,
		NumForcedGC:   m.NumForcedGC,
		GCCPUFraction: m.GCCPUFraction,
		EnableGC:      m.EnableGC,
		DebugGC:       m.DebugGC,
		Timestamp:     time.Now().Format(time.RFC3339),
	}

	// 如果请求包含 by_size 参数，添加按大小分类的统计
	if r.URL.Query().Get("by_size") == "true" {
		stats.BySize = make([]SizeClassStats, len(m.BySize))
		for i, sizeClass := range m.BySize {
			stats.BySize[i] = SizeClassStats{
				Size:    sizeClass.Size,
				Mallocs: sizeClass.Mallocs,
				Frees:   sizeClass.Frees,
			}
		}
	}

	s.respondJSON(w, http.StatusOK, stats)
}

// handleGetMemoryProfile 获取内存 profile (pprof 格式)
func (s *ManagementAPIServer) handleGetMemoryProfile(w http.ResponseWriter, r *http.Request) {
	// 获取 pprof 参数
	debug := r.URL.Query().Get("debug")
	debugLevel := 0
	if debug != "" {
		var err error
		debugLevel, err = strconv.Atoi(debug)
		if err != nil || debugLevel < 0 || debugLevel > 2 {
			debugLevel = 0
		}
	}

	// 获取 heap profile
	profile := pprof.Lookup("heap")
	if profile == nil {
		s.respondError(w, http.StatusInternalServerError, "Failed to get heap profile")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if debugLevel > 0 {
		err := profile.WriteTo(w, debugLevel)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write profile: %v", err))
			return
		}
	} else {
		// 使用 JSON 格式返回
		var buf bytes.Buffer
		err := profile.WriteTo(&buf, 0)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write profile: %v", err))
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]string{
			"profile": buf.String(),
		})
	}
}

// handleForceGC 强制执行 GC
func (s *ManagementAPIServer) handleForceGC(w http.ResponseWriter, r *http.Request) {
	before := runtime.NumGoroutine()
	var mBefore runtime.MemStats
	runtime.ReadMemStats(&mBefore)

	// 执行 GC
	runtime.GC()

	// 等待 GC 完成
	time.Sleep(100 * time.Millisecond)

	after := runtime.NumGoroutine()
	var mAfter runtime.MemStats
	runtime.ReadMemStats(&mAfter)

	result := map[string]interface{}{
		"success": true,
		"before": map[string]interface{}{
			"goroutines":    before,
			"heap_alloc_mb": float64(mBefore.HeapAlloc) / 1024 / 1024,
			"heap_sys_mb":   float64(mBefore.HeapSys) / 1024 / 1024,
			"heap_objects":  mBefore.HeapObjects,
		},
		"after": map[string]interface{}{
			"goroutines":    after,
			"heap_alloc_mb": float64(mAfter.HeapAlloc) / 1024 / 1024,
			"heap_sys_mb":   float64(mAfter.HeapSys) / 1024 / 1024,
			"heap_objects":  mAfter.HeapObjects,
		},
		"freed": map[string]interface{}{
			"heap_alloc_mb": float64(mBefore.HeapAlloc-mAfter.HeapAlloc) / 1024 / 1024,
			"heap_objects":  int64(mBefore.HeapObjects) - int64(mAfter.HeapObjects),
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	s.respondJSON(w, http.StatusOK, result)
}

// handleGetMemoryLeakReport 生成内存泄漏报告
func (s *ManagementAPIServer) handleGetMemoryLeakReport(w http.ResponseWriter, r *http.Request) {
	// 获取两次内存快照（间隔1秒）
	var m1, m2 runtime.MemStats
	
	runtime.ReadMemStats(&m1)
	time.Sleep(1 * time.Second)
	runtime.ReadMemStats(&m2)

	// 计算内存增长
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	heapObjectsGrowth := int64(m2.HeapObjects) - int64(m1.HeapObjects)
	totalAllocGrowth := int64(m2.TotalAlloc) - int64(m1.TotalAlloc)

	// 获取 goroutine 数量
	goroutines1 := runtime.NumGoroutine()
	time.Sleep(100 * time.Millisecond)
	goroutines2 := runtime.NumGoroutine()

	report := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"memory": map[string]interface{}{
			"heap_alloc_growth_bytes":    heapGrowth,
			"heap_alloc_growth_mb":       float64(heapGrowth) / 1024 / 1024,
			"heap_objects_growth":        heapObjectsGrowth,
			"total_alloc_growth_bytes":   totalAllocGrowth,
			"total_alloc_growth_mb":      float64(totalAllocGrowth) / 1024 / 1024,
			"current_heap_alloc_mb":      float64(m2.HeapAlloc) / 1024 / 1024,
			"current_heap_sys_mb":        float64(m2.HeapSys) / 1024 / 1024,
			"current_heap_objects":       m2.HeapObjects,
			"heap_idle_mb":               float64(m2.HeapIdle) / 1024 / 1024,
			"heap_inuse_mb":              float64(m2.HeapInuse) / 1024 / 1024,
			"potential_leak":             heapGrowth > 1024*1024, // 如果1秒内增长超过1MB，可能泄漏
		},
		"goroutines": map[string]interface{}{
			"count":           goroutines2,
			"growth":          goroutines2 - goroutines1,
			"potential_leak": goroutines2 > goroutines1 && goroutines2 > 100, // 如果goroutine持续增长且超过100，可能泄漏
		},
		"gc": map[string]interface{}{
			"num_gc":          m2.NumGC,
			"pause_total_ms":  float64(m2.PauseTotalNs) / 1000000,
			"gc_cpu_fraction": m2.GCCPUFraction,
		},
		"recommendations": generateLeakRecommendations(heapGrowth, heapObjectsGrowth, goroutines2-goroutines1),
	}

	s.respondJSON(w, http.StatusOK, report)
}

// generateLeakRecommendations 生成泄漏建议
func generateLeakRecommendations(heapGrowth int64, heapObjectsGrowth int64, goroutineGrowth int) []string {
	var recommendations []string

	if heapGrowth > 10*1024*1024 { // 超过10MB
		recommendations = append(recommendations, "⚠️ 内存增长过快，可能存在内存泄漏，建议检查未释放的资源")
	}

	if heapObjectsGrowth > 10000 {
		recommendations = append(recommendations, "⚠️ 堆对象数量增长过快，可能存在对象泄漏")
	}

	if goroutineGrowth > 0 {
		recommendations = append(recommendations, "⚠️ Goroutine 数量在增长，可能存在 goroutine 泄漏")
	}

	if heapGrowth > 0 && heapObjectsGrowth == 0 {
		recommendations = append(recommendations, "ℹ️ 内存增长但对象数量未增长，可能是大对象分配")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "✅ 未检测到明显的资源泄漏迹象")
	}

	return recommendations
}


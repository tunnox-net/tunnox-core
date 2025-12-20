package api

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/utils"
)

// PProfCapture pprof 自动抓取器
type PProfCapture struct {
	*dispose.ResourceBase
	config  *PProfConfig
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	running bool
}

// NewPProfCapture 创建 pprof 抓取器
func NewPProfCapture(ctx context.Context, config *PProfConfig) *PProfCapture {
	captureCtx, cancel := context.WithCancel(ctx)
	capture := &PProfCapture{
		ResourceBase: dispose.NewResourceBase("PProfCapture"),
		config:       config,
		ctx:          captureCtx,
		cancel:       cancel,
	}
	capture.Initialize(captureCtx)
	capture.AddCleanHandler(capture.onClose)
	return capture
}

// Start 启动自动抓取
func (p *PProfCapture) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("pprof capture is already running")
	}

	if !p.config.Enabled || !p.config.AutoCapture {
		return nil
	}

	// 确保数据目录存在
	if err := p.ensureDataDir(); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	p.running = true
	go p.captureLoop()

	corelog.Infof("PProfCapture: started, saving profiles to %s (retention: %d minutes)", p.config.DataDir, p.config.Retention)
	return nil
}

// Stop 停止自动抓取
func (p *PProfCapture) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.running = false
	p.cancel()
	corelog.Infof("PProfCapture: stopped")
	return nil
}

// onClose 资源清理
func (p *PProfCapture) onClose() error {
	return p.Stop()
}

// ensureDataDir 确保数据目录存在
func (p *PProfCapture) ensureDataDir() error {
	if p.config.DataDir == "" {
		return fmt.Errorf("pprof data directory is not configured")
	}

	// 展开路径（支持 ~ 和相对路径）
	expandedPath, err := utils.ExpandPath(p.config.DataDir)
	if err != nil {
		return fmt.Errorf("failed to expand path %q: %w", p.config.DataDir, err)
	}

	p.config.DataDir = expandedPath

	// 创建目录
	if err := os.MkdirAll(p.config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", p.config.DataDir, err)
	}

	return nil
}

// captureLoop 抓取循环（每分钟执行一次）
func (p *PProfCapture) captureLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// 立即执行一次
	p.captureProfiles()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.captureProfiles()
			p.cleanupOldFiles()
		}
	}
}

// captureProfiles 抓取所有 pprof 数据
func (p *PProfCapture) captureProfiles() {
	timestamp := time.Now().Format("20060102-150405")
	basePath := filepath.Join(p.config.DataDir, timestamp)

	// 抓取 heap profile
	if err := p.captureHeap(basePath + ".heap"); err != nil {
		corelog.Warnf("PProfCapture: failed to capture heap profile: %v", err)
	}

	// 抓取 goroutine profile
	if err := p.captureGoroutine(basePath + ".goroutine"); err != nil {
		corelog.Warnf("PProfCapture: failed to capture goroutine profile: %v", err)
	}

	// 抓取 allocs profile
	if err := p.captureAllocs(basePath + ".allocs"); err != nil {
		corelog.Warnf("PProfCapture: failed to capture allocs profile: %v", err)
	}

	// 抓取 block profile
	if err := p.captureBlock(basePath + ".block"); err != nil {
		corelog.Warnf("PProfCapture: failed to capture block profile: %v", err)
	}

	// 抓取 mutex profile
	if err := p.captureMutex(basePath + ".mutex"); err != nil {
		corelog.Warnf("PProfCapture: failed to capture mutex profile: %v", err)
	}

	corelog.Debugf("PProfCapture: captured profiles at %s", timestamp)
}

// captureHeap 抓取堆内存 profile
func (p *PProfCapture) captureHeap(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	runtime.GC() // 触发 GC 以获得更准确的堆信息
	return pprof.WriteHeapProfile(f)
}

// captureGoroutine 抓取 goroutine profile
func (p *PProfCapture) captureGoroutine(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	profile := pprof.Lookup("goroutine")
	if profile == nil {
		return fmt.Errorf("goroutine profile not found")
	}
	return profile.WriteTo(f, 0)
}

// captureAllocs 抓取内存分配 profile
func (p *PProfCapture) captureAllocs(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	profile := pprof.Lookup("allocs")
	if profile == nil {
		return fmt.Errorf("allocs profile not found")
	}
	return profile.WriteTo(f, 0)
}

// captureBlock 抓取阻塞 profile
func (p *PProfCapture) captureBlock(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	profile := pprof.Lookup("block")
	if profile == nil {
		return fmt.Errorf("block profile not found")
	}
	return profile.WriteTo(f, 0)
}

// captureMutex 抓取互斥锁 profile
func (p *PProfCapture) captureMutex(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	profile := pprof.Lookup("mutex")
	if profile == nil {
		return fmt.Errorf("mutex profile not found")
	}
	return profile.WriteTo(f, 0)
}

// cleanupOldFiles 清理超过保留时间的文件
func (p *PProfCapture) cleanupOldFiles() {
	retentionDuration := time.Duration(p.config.Retention) * time.Minute
	if retentionDuration <= 0 {
		retentionDuration = 10 * time.Minute // 默认10分钟
	}

	cutoffTime := time.Now().Add(-retentionDuration)

	entries, err := os.ReadDir(p.config.DataDir)
	if err != nil {
		corelog.Warnf("PProfCapture: failed to read data directory: %v", err)
		return
	}

	removedCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 只处理 pprof 相关文件
		if !isPProfFile(entry.Name()) {
			continue
		}

		// 如果文件修改时间早于截止时间，删除它
		if info.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(p.config.DataDir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				corelog.Warnf("PProfCapture: failed to remove old file %s: %v", filePath, err)
			} else {
				removedCount++
			}
		}
	}

	if removedCount > 0 {
		corelog.Debugf("PProfCapture: cleaned up %d old profile files", removedCount)
	}
}

// isPProfFile 判断是否为 pprof 文件
func isPProfFile(filename string) bool {
	ext := filepath.Ext(filename)
	return ext == ".heap" || ext == ".goroutine" || ext == ".allocs" ||
		ext == ".block" || ext == ".mutex" || ext == ".profile"
}


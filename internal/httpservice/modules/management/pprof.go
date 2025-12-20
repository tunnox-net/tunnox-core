package management

import (
	"context"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	rpprof "runtime/pprof"
	"sync"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"

	"github.com/gorilla/mux"
)

// PProfHandler pprof 处理器
type PProfHandler struct {
	enabled bool
}

// NewPProfHandler 创建 pprof 处理器
func NewPProfHandler(enabled bool) *PProfHandler {
	return &PProfHandler{enabled: enabled}
}

// RegisterRoutes 注册 pprof 路由
func (h *PProfHandler) RegisterRoutes(router *mux.Router, authMiddleware mux.MiddlewareFunc) {
	if !h.enabled {
		return
	}

	pprofRouter := router.PathPrefix("/tunnox/v1/debug/pprof").Subrouter()

	if authMiddleware != nil {
		pprofRouter.Use(authMiddleware)
	}

	pprofRouter.HandleFunc("/", pprof.Index)
	pprofRouter.HandleFunc("/cmdline", pprof.Cmdline)
	pprofRouter.HandleFunc("/profile", pprof.Profile)
	pprofRouter.HandleFunc("/symbol", pprof.Symbol)
	pprofRouter.HandleFunc("/trace", pprof.Trace)
	pprofRouter.Handle("/goroutine", pprof.Handler("goroutine"))
	pprofRouter.Handle("/heap", pprof.Handler("heap"))
	pprofRouter.Handle("/threadcreate", pprof.Handler("threadcreate"))
	pprofRouter.Handle("/block", pprof.Handler("block"))
	pprofRouter.Handle("/allocs", pprof.Handler("allocs"))
	pprofRouter.Handle("/mutex", pprof.Handler("mutex"))
}

// PProfCapture pprof 自动抓取器
type PProfCapture struct {
	ctx      context.Context
	cancel   context.CancelFunc
	config   *httpservice.PProfConfig
	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
}

// NewPProfCapture 创建 pprof 自动抓取器
func NewPProfCapture(parentCtx context.Context, config *httpservice.PProfConfig) *PProfCapture {
	ctx, cancel := context.WithCancel(parentCtx)
	return &PProfCapture{
		ctx:      ctx,
		cancel:   cancel,
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start 启动自动抓取
func (c *PProfCapture) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	// 确保目录存在
	if err := os.MkdirAll(c.config.DataDir, 0755); err != nil {
		return err
	}

	c.running = true

	go c.captureLoop()

	corelog.Infof("PProfCapture: started, data_dir=%s, retention=%d min", c.config.DataDir, c.config.Retention)
	return nil
}

// Stop 停止自动抓取
func (c *PProfCapture) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.cancel()
	close(c.stopChan)
	c.running = false

	corelog.Infof("PProfCapture: stopped")
	return nil
}

// captureLoop 抓取循环
func (c *PProfCapture) captureLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.capture()
			c.cleanup()
		}
	}
}

// capture 执行一次抓取
func (c *PProfCapture) capture() {
	timestamp := time.Now().Format("20060102_150405")

	// 抓取 heap profile
	heapFile := filepath.Join(c.config.DataDir, "heap_"+timestamp+".pprof")
	if f, err := os.Create(heapFile); err == nil {
		runtime.GC()
		rpprof.WriteHeapProfile(f)
		f.Close()
	}

	// 抓取 goroutine profile
	goroutineFile := filepath.Join(c.config.DataDir, "goroutine_"+timestamp+".pprof")
	if f, err := os.Create(goroutineFile); err == nil {
		rpprof.Lookup("goroutine").WriteTo(f, 0)
		f.Close()
	}
}

// cleanup 清理过期文件
func (c *PProfCapture) cleanup() {
	retention := time.Duration(c.config.Retention) * time.Minute
	cutoff := time.Now().Add(-retention)

	entries, err := os.ReadDir(c.config.DataDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(c.config.DataDir, entry.Name()))
		}
	}
}

package udp

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

// Config 描述 UDP 接入的全局配置
type Config struct {
	Enabled   bool             `yaml:"enabled"`
	Listeners []ListenerConfig `yaml:"listeners"`
}

// ListenerConfig 描述单个 UDP Listener
type ListenerConfig struct {
	Name         string `yaml:"name"`
	Address      string `yaml:"address"`
	MappingID    string `yaml:"mapping_id"`
	IdleTimeout  int    `yaml:"idle_timeout"`  // 秒
	MaxSessions  int    `yaml:"max_sessions"`  // 0 = 不限制
	FrameBacklog int    `yaml:"frame_backlog"` // 每个会话保存的帧数量
}

// Manager 管理所有 UDP 接入监听
type Manager struct {
	cfg     Config
	session *session.SessionManager
	cloud   session.CloudControlAPI

	listeners []*listener
}

// NewManager 创建 UDP Ingress 管理器
func NewManager(cfg Config, sess *session.SessionManager, cloud session.CloudControlAPI) *Manager {
	return &Manager{
		cfg:     cfg,
		session: sess,
		cloud:   cloud,
	}
}

// Start 启动所有配置的 UDP 监听
func (m *Manager) Start(ctx context.Context) error {
	if !m.cfg.Enabled {
		utils.Info("UDP ingress disabled by configuration")
		return nil
	}
	if m.cloud == nil {
		return fmt.Errorf("cloud control api not configured")
	}
	for _, lc := range m.cfg.Listeners {
		listener, err := newListener(ctx, lc, m.session, m.cloud)
		if err != nil {
			return err
		}
		m.listeners = append(m.listeners, listener)
	}
	return nil
}

// Close 停止所有 UDP 监听
func (m *Manager) Close() error {
	for _, l := range m.listeners {
		l.close()
	}
	m.listeners = nil
	return nil
}

// listener 表示单个 UDP 监听器
type listener struct {
	cfg     ListenerConfig
	session *session.SessionManager
	cloud   session.CloudControlAPI

	conn   *net.UDPConn
	ctx    context.Context
	cancel context.CancelFunc

	sessions map[string]*ingressSession
	mu       sync.RWMutex
}

func newListener(parent context.Context, cfg ListenerConfig, sess *session.SessionManager, cloud session.CloudControlAPI) (*listener, error) {
	addr, err := net.ResolveUDPAddr("udp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("udp ingress resolve %s: %w", cfg.Address, err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("udp ingress listen %s: %w", cfg.Address, err)
	}

	ctx, cancel := context.WithCancel(parent)
	l := &listener{
		cfg:      cfg,
		session:  sess,
		cloud:    cloud,
		conn:     conn,
		ctx:      ctx,
		cancel:   cancel,
		sessions: make(map[string]*ingressSession),
	}

	go l.run()
	go l.cleanupLoop()

	utils.Infof("UDP ingress listener up: %s (%s)", cfg.Name, cfg.Address)
	return l, nil
}

func (l *listener) run() {
	buf := make([]byte, 65535)
	for {
		if err := l.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			utils.Errorf("UDP ingress set deadline: %v", err)
			return
		}

		n, addr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			utils.Errorf("UDP ingress read error: %v", err)
			continue
		}

		if n == 0 {
			continue
		}

		l.handlePacket(addr, buf[:n])
	}
}

func (l *listener) handlePacket(addr *net.UDPAddr, payload []byte) {
	session, err := l.getOrCreateSession(addr)
	if err != nil {
		utils.Errorf("UDP ingress session error (%s): %v", addr, err)
		return
	}

	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)

	if err := session.pushFrame(frame); err != nil {
		utils.Debugf("UDP ingress drop packet from %s: %v", addr, err)
		session.close()
		l.removeSession(addr.String())
	}
}

func (l *listener) getOrCreateSession(addr *net.UDPAddr) (*ingressSession, error) {
	key := addr.String()

	l.mu.RLock()
	if sess, ok := l.sessions[key]; ok {
		l.mu.RUnlock()
		return sess, nil
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	if sess, ok := l.sessions[key]; ok {
		return sess, nil
	}

	if l.cfg.MaxSessions > 0 && len(l.sessions) >= l.cfg.MaxSessions {
		return nil, fmt.Errorf("session limit reached")
	}

	if _, err := l.lookupMapping(); err != nil {
		return nil, err
	}

	conn := newPipeConn(l.conn, addr, l.cfg.FrameBacklog)
	tunnelID, err := l.session.StartServerTunnel(l.cfg.MappingID, conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	sess := &ingressSession{
		remote:     addr,
		conn:       conn,
		tunnelID:   tunnelID,
		lastActive: time.Now(),
		listener:   l,
	}
	l.sessions[key] = sess
	go sess.watch()

	utils.Infof("UDP ingress session created: %s (tunnel=%s)", addr, tunnelID)
	return sess, nil
}

func (l *listener) lookupMapping() (*models.PortMapping, error) {
	value, err := l.cloud.GetPortMapping(l.cfg.MappingID)
	if err != nil {
		return nil, fmt.Errorf("mapping %s not found: %w", l.cfg.MappingID, err)
	}
	mapping, ok := value.(*models.PortMapping)
	if !ok {
		return nil, fmt.Errorf("invalid mapping type for %s", l.cfg.MappingID)
	}
	return mapping, nil
}

func (l *listener) cleanupLoop() {
	idle := time.Duration(l.cfg.IdleTimeout) * time.Second
	if idle <= 0 {
		idle = 60 * time.Second
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			l.mu.Lock()
			for key, sess := range l.sessions {
				if now.Sub(sess.lastActive) >= idle {
					utils.Debugf("UDP ingress session idle timeout: %s", sess.remote)
					sess.close()
					delete(l.sessions, key)
				}
			}
			l.mu.Unlock()
		}
	}
}

func (l *listener) removeSession(key string) {
	l.mu.Lock()
	delete(l.sessions, key)
	l.mu.Unlock()
}

func (l *listener) close() {
	l.cancel()
	l.conn.Close()

	l.mu.Lock()
	for key, sess := range l.sessions {
		sess.close()
		delete(l.sessions, key)
	}
	l.mu.Unlock()
}

// ingressSession 表示单个远端 UDP 会话
type ingressSession struct {
	remote     *net.UDPAddr
	conn       *pipeConn
	tunnelID   string
	lastActive time.Time
	listener   *listener
	once       sync.Once
}

func (s *ingressSession) pushFrame(frame []byte) error {
	s.lastActive = time.Now()
	return s.conn.Push(frame)
}

func (s *ingressSession) close() {
	s.once.Do(func() {
		s.conn.Close()
		utils.Infof("UDP ingress session closed: %s (tunnel=%s)", s.remote, s.tunnelID)
	})
}

func (s *ingressSession) watch() {
	<-s.conn.Done()
	s.close()
	s.listener.removeSession(s.remote.String())
}

# 连接安全加固方案

## 当前安全漏洞分析

### 🔴 指令通道（Control Connection）安全问题

#### 1. 匿名客户端风险
```go
// 当前代码（存在风险）
if req.ClientID == 0 {
    anonClient, _ := h.cloudControl.GenerateAnonymousCredentials()
    clientID = anonClient.ID  // ❌ 无限制生成
}
```

**风险**：
- ❌ 无速率限制 → DDoS攻击
- ❌ 无IP验证 → 任意IP可注册
- ❌ 无配额限制 → 资源耗尽
- ❌ 无验证码 → 自动化攻击

#### 2. 注册客户端认证不足
```go
// 当前代码（安全性不足）
authResp, err := h.cloudControl.Authenticate(&models.AuthRequest{
    ClientID: req.ClientID,
    AuthCode: req.Token,  // ❌ 仅验证Token
})
```

**缺失的安全措施**：
- ❌ 无密码复杂度要求
- ❌ 无多因子认证（MFA）
- ❌ 无暴力破解防护
- ❌ 无IP白名单/黑名单
- ❌ 无重复登录检测
- ❌ Token明文传输（如未启用TLS）

#### 3. 会话管理问题
```go
// 当前代码
conn.ClientID = clientID
conn.Authenticated = true  // ❌ 无会话超时
```

**风险**：
- ❌ 无会话超时 → 长期有效
- ❌ 无并发连接限制 → 资源耗尽
- ❌ 无会话绑定 → 会话劫持
- ❌ 无活动监控 → 异常行为无法检测

### 🔴 映射通道（Tunnel Connection）安全问题

#### 1. 弱认证机制
```go
// 当前代码（仅验证SecretKey）
if mapping.SecretKey != req.SecretKey {
    return fmt.Errorf("invalid secret key")
}
```

**风险**：
- ❌ SecretKey可能泄露
- ❌ 无来源验证 → 任意客户端可用
- ❌ 无权限检查 → 越权访问
- ❌ 无时间限制 → 永久有效

#### 2. 缺失的验证
```go
// 当前代码
// ✅ 隧道连接不需要验证ClientID，只验证SecretKey
// ❌ 这是错误的假设！
```

**应该验证**：
- ⭐ 必须验证ClientID（确认发起者身份）
- ⭐ 必须验证权限（是否有权访问此映射）
- ⭐ 必须验证来源（防止中间人）
- ⭐ 必须限制速率（防止滥用）

## 安全加固方案

### 🛡️ Layer 1: 传输层安全（基础）

#### 1.1 强制TLS/DTLS
```go
// SecurityConfig 安全配置
type SecurityConfig struct {
    // ⭐ 强制TLS
    ForceTLS bool `yaml:"force_tls"` // 生产环境必须true
    
    // ⭐ TLS配置
    TLS struct {
        MinVersion   string   `yaml:"min_version"`   // "1.2" 或 "1.3"
        CipherSuites []string `yaml:"cipher_suites"` // 允许的密码套件
        RequireClientCert bool `yaml:"require_client_cert"` // 是否要求客户端证书
    } `yaml:"tls"`
}

// 验证TLS连接
func (s *Server) validateTLSConnection(conn net.Conn) error {
    tlsConn, ok := conn.(*tls.Conn)
    if !ok {
        if s.config.Security.ForceTLS {
            return ErrTLSRequired // ⭐ 拒绝非TLS连接
        }
        return nil
    }
    
    // ⭐ 验证TLS版本
    state := tlsConn.ConnectionState()
    if state.Version < tls.VersionTLS12 {
        return ErrTLSVersionTooLow
    }
    
    // ⭐ 验证客户端证书（如果要求）
    if s.config.Security.TLS.RequireClientCert {
        if len(state.PeerCertificates) == 0 {
            return ErrClientCertRequired
        }
        
        // 提取证书信息
        cert := state.PeerCertificates[0]
        clientIDFromCert := extractClientIDFromCert(cert)
        
        // 验证证书未被吊销
        if isClientCertRevoked(clientIDFromCert) {
            return ErrClientCertRevoked
        }
    }
    
    return nil
}
```

#### 1.2 TLS指纹绑定
```go
// 提取TLS指纹（用于会话绑定）
func extractTLSFingerprint(conn *tls.Conn) string {
    state := conn.ConnectionState()
    
    // 计算指纹：TLS版本 + 密码套件 + 证书哈希
    fingerprint := fmt.Sprintf("%d:%d:%x",
        state.Version,
        state.CipherSuite,
        sha256.Sum256(state.PeerCertificates[0].Raw),
    )
    
    return base64.StdEncoding.EncodeToString([]byte(fingerprint))
}

// 验证TLS指纹（防止会话劫持）
func (h *ServerAuthHandler) validateTLSFingerprint(conn *session.ClientConnection, storedFingerprint string) error {
    // 从底层连接提取TLS指纹
    currentFingerprint := extractTLSFingerprint(conn.TLSConn)
    
    if currentFingerprint != storedFingerprint {
        return ErrTLSFingerprintMismatch // ⭐ TLS指纹变化，可能被劫持
    }
    
    return nil
}
```

### 🛡️ Layer 2: 身份认证增强（指令通道）

#### 2.1 匿名客户端限制
```go
// AnonymousClientConfig 匿名客户端配置
type AnonymousClientConfig struct {
    Enabled            bool          `yaml:"enabled"`               // 是否允许匿名
    MaxPerIP           int           `yaml:"max_per_ip"`            // 每IP最大匿名数（如3个）
    MaxPerMinute       int           `yaml:"max_per_minute"`        // 每分钟最大注册数
    RequireCaptcha     bool          `yaml:"require_captcha"`       // 是否需要验证码
    DefaultQuota       QuotaConfig   `yaml:"default_quota"`         // 默认配额
    TTL                time.Duration `yaml:"ttl"`                   // 匿名客户端有效期（如24小时）
}

type QuotaConfig struct {
    MaxMappings     int `yaml:"max_mappings"`      // 最大映射数（如1个）
    MaxBandwidth    int `yaml:"max_bandwidth"`     // 最大带宽（MB/s）
    MaxConnections  int `yaml:"max_connections"`   // 最大并发连接数
}

// 增强的匿名客户端生成
func (h *ServerAuthHandler) handleAnonymousClient(conn *session.ClientConnection, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
    // ⭐ 1. 检查是否允许匿名
    if !h.config.AnonymousClient.Enabled {
        return nil, ErrAnonymousNotAllowed
    }
    
    // ⭐ 2. 获取客户端IP
    clientIP := extractClientIP(conn)
    
    // ⭐ 3. 验证码检查（如果启用）
    if h.config.AnonymousClient.RequireCaptcha {
        if !validateCaptcha(req.CaptchaToken) {
            return nil, ErrInvalidCaptcha
        }
    }
    
    // ⭐ 4. 检查IP速率限制
    if !h.anonymousRateLimiter.AllowIP(clientIP) {
        utils.Warnf("IP %s exceeded anonymous client creation rate limit", clientIP)
        return nil, ErrRateLimitExceeded
    }
    
    // ⭐ 5. 检查每IP配额
    count, _ := h.getAnonymousCountByIP(clientIP)
    if count >= h.config.AnonymousClient.MaxPerIP {
        utils.Warnf("IP %s exceeded max anonymous clients (%d)", clientIP, count)
        return nil, ErrIPQuotaExceeded
    }
    
    // ⭐ 6. 生成匿名客户端（带配额和TTL）
    anonClient, err := h.cloudControl.GenerateAnonymousCredentials()
    if err != nil {
        return nil, err
    }
    
    // ⭐ 7. 设置配额和过期时间
    h.setClientQuota(anonClient.ID, h.config.AnonymousClient.DefaultQuota)
    h.setClientTTL(anonClient.ID, h.config.AnonymousClient.TTL)
    
    // ⭐ 8. 记录IP关联（用于配额检查）
    h.associateClientWithIP(anonClient.ID, clientIP)
    
    // ⭐ 9. 安全审计
    h.logSecurityEvent(&SecurityEvent{
        Type:      "anonymous_client_created",
        ClientID:  anonClient.ID,
        IPAddress: clientIP,
        Success:   true,
    })
    
    return &packet.HandshakeResponse{
        Success: true,
        Message: fmt.Sprintf("Anonymous client created: %d (expires in %s)", 
            anonClient.ID, h.config.AnonymousClient.TTL),
    }, nil
}
```

#### 2.2 注册客户端认证增强
```go
// 增强的认证流程
func (h *ServerAuthHandler) handleRegisteredClient(conn *session.ClientConnection, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
    clientIP := extractClientIP(conn)
    
    // ⭐ 1. IP黑名单检查
    if h.isIPBlocked(clientIP) {
        utils.Warnf("Blocked IP %s attempted to connect", clientIP)
        return nil, ErrIPBlocked
    }
    
    // ⭐ 2. 客户端黑名单检查
    if h.isClientBlocked(req.ClientID) {
        utils.Warnf("Blocked client %d attempted to connect", req.ClientID)
        return nil, ErrClientBlocked
    }
    
    // ⭐ 3. 暴力破解防护
    if !h.bruteForceProtector.AllowAttempt(req.ClientID, clientIP) {
        utils.Warnf("Client %d from IP %s exceeded authentication attempts", req.ClientID, clientIP)
        return nil, ErrTooManyAttempts
    }
    
    // ⭐ 4. 验证AuthCode/Token
    authResp, err := h.cloudControl.Authenticate(&models.AuthRequest{
        ClientID: req.ClientID,
        AuthCode: req.Token,
    })
    
    if err != nil || !authResp.Success {
        // ⭐ 记录失败尝试（用于暴力破解检测）
        h.bruteForceProtector.RecordFailure(req.ClientID, clientIP)
        
        // ⭐ 安全审计
        h.logSecurityEvent(&SecurityEvent{
            Type:        "authentication_failed",
            ClientID:    req.ClientID,
            IPAddress:   clientIP,
            Success:     false,
            ErrorReason: "invalid_credentials",
            RiskScore:   5,
        })
        
        return nil, ErrAuthenticationFailed
    }
    
    // ⭐ 5. 重置失败计数（认证成功）
    h.bruteForceProtector.ResetFailures(req.ClientID, clientIP)
    
    // ⭐ 6. IP白名单检查（可选，高安全性客户端）
    if h.clientHasIPWhitelist(req.ClientID) {
        if !h.isIPWhitelisted(req.ClientID, clientIP) {
            utils.Warnf("Client %d connected from non-whitelisted IP %s", req.ClientID, clientIP)
            // 可选：拒绝或发送MFA请求
            return nil, ErrIPNotWhitelisted
        }
    }
    
    // ⭐ 7. 多因子认证（MFA）检查
    if h.clientRequiresMFA(req.ClientID) {
        if !h.validateMFA(req.ClientID, req.MFAToken) {
            return nil, ErrMFARequired
        }
    }
    
    // ⭐ 8. 检查重复连接
    existingConn := h.sessionMgr.GetControlConnectionByClientID(req.ClientID)
    if existingConn != nil {
        // ⭐ 踢掉旧连接（记录可疑活动）
        utils.Warnf("Client %d has existing connection, kicking old one", req.ClientID)
        h.sessionMgr.KickOldControlConnection(req.ClientID, conn.ConnID)
        
        // ⭐ 安全审计（可疑的重复登录）
        h.logSecurityEvent(&SecurityEvent{
            Type:      "duplicate_login",
            ClientID:  req.ClientID,
            IPAddress: clientIP,
            Success:   true,
            RiskScore: 7, // 中高风险
        })
    }
    
    // ⭐ 9. TLS指纹存储（用于后续验证）
    if tlsConn, ok := conn.RawConn.(*tls.Conn); ok {
        fingerprint := extractTLSFingerprint(tlsConn)
        h.storeClientTLSFingerprint(req.ClientID, fingerprint)
    }
    
    // ⭐ 10. 会话Token生成（用于后续请求）
    sessionToken := h.generateSessionToken(req.ClientID, clientIP)
    h.storeSessionToken(req.ClientID, sessionToken)
    
    // ⭐ 11. 安全审计（成功登录）
    h.logSecurityEvent(&SecurityEvent{
        Type:      "authentication_success",
        ClientID:  req.ClientID,
        IPAddress: clientIP,
        Success:   true,
        RiskScore: 1,
    })
    
    return &packet.HandshakeResponse{
        Success:      true,
        Message:      "Authentication successful",
        SessionToken: sessionToken, // ⭐ 返回会话Token
    }, nil
}
```

#### 2.3 暴力破解防护
```go
// BruteForceProtector 暴力破解防护器
type BruteForceProtector struct {
    attempts map[string]*AttemptRecord // "clientID:IP" -> 记录
    mu       sync.RWMutex
}

type AttemptRecord struct {
    FailureCount int
    FirstFailure time.Time
    LastFailure  time.Time
    BlockedUntil time.Time
}

const (
    MaxFailures = 5               // 最多5次失败
    BlockDuration = 15 * time.Minute // 封禁15分钟
    WindowDuration = 5 * time.Minute  // 时间窗口5分钟
)

func (p *BruteForceProtector) AllowAttempt(clientID int64, ip string) bool {
    key := fmt.Sprintf("%d:%s", clientID, ip)
    
    p.mu.RLock()
    record, exists := p.attempts[key]
    p.mu.RUnlock()
    
    if !exists {
        return true // 首次尝试
    }
    
    // ⭐ 检查是否仍在封禁期
    if time.Now().Before(record.BlockedUntil) {
        return false
    }
    
    // ⭐ 检查时间窗口
    if time.Since(record.FirstFailure) > WindowDuration {
        // 窗口过期，重置
        p.mu.Lock()
        delete(p.attempts, key)
        p.mu.Unlock()
        return true
    }
    
    // ⭐ 检查失败次数
    if record.FailureCount >= MaxFailures {
        return false
    }
    
    return true
}

func (p *BruteForceProtector) RecordFailure(clientID int64, ip string) {
    key := fmt.Sprintf("%d:%s", clientID, ip)
    
    p.mu.Lock()
    defer p.mu.Unlock()
    
    record, exists := p.attempts[key]
    if !exists {
        p.attempts[key] = &AttemptRecord{
            FailureCount: 1,
            FirstFailure: time.Now(),
            LastFailure:  time.Now(),
        }
        return
    }
    
    record.FailureCount++
    record.LastFailure = time.Now()
    
    // ⭐ 超过阈值，封禁
    if record.FailureCount >= MaxFailures {
        record.BlockedUntil = time.Now().Add(BlockDuration)
        utils.Warnf("Client %d from IP %s blocked for %s due to brute force",
            clientID, ip, BlockDuration)
    }
}
```

### 🛡️ Layer 3: 映射通道认证增强

#### 3.1 完整的隧道认证
```go
// 增强的隧道打开处理
func (h *ServerTunnelHandler) HandleTunnelOpen(conn *session.ClientConnection, req *packet.TunnelOpenRequest) error {
    clientIP := extractClientIP(conn)
    
    // ⭐ 1. 获取映射信息
    mapping, err := h.cloudControl.GetPortMapping(req.MappingID)
    if err != nil {
        utils.Errorf("Mapping not found: %s", req.MappingID)
        return ErrMappingNotFound
    }
    
    // ⭐ 2. 验证SecretKey（基础认证）
    if !hmac.Equal([]byte(mapping.SecretKey), []byte(req.SecretKey)) {
        utils.Warnf("Invalid secret key for mapping %s from IP %s", req.MappingID, clientIP)
        h.logSecurityEvent(&SecurityEvent{
            Type:        "tunnel_auth_failed",
            MappingID:   req.MappingID,
            IPAddress:   clientIP,
            Success:     false,
            ErrorReason: "invalid_secret_key",
            RiskScore:   8, // 高风险
        })
        return ErrInvalidSecretKey
    }
    
    // ⭐ 3. 验证客户端身份（必须）
    // 从连接中提取ClientID（如果有握手）
    clientID := conn.ClientID
    if clientID == 0 {
        // ⭐ 未经握手的连接，必须通过其他方式验证
        // 例如：从TLS证书提取ClientID
        if tlsConn, ok := conn.RawConn.(*tls.Conn); ok {
            state := tlsConn.ConnectionState()
            if len(state.PeerCertificates) > 0 {
                clientID = extractClientIDFromCert(state.PeerCertificates[0])
            }
        }
        
        if clientID == 0 {
            return ErrClientIDRequired // ⭐ 必须有客户端身份
        }
    }
    
    // ⭐ 4. 权限验证（关键）
    if !h.hasPermission(clientID, mapping) {
        utils.Warnf("Client %d has no permission for mapping %s", clientID, req.MappingID)
        h.logSecurityEvent(&SecurityEvent{
            Type:        "tunnel_permission_denied",
            ClientID:    clientID,
            MappingID:   req.MappingID,
            IPAddress:   clientIP,
            Success:     false,
            ErrorReason: "permission_denied",
            RiskScore:   9, // 极高风险（越权尝试）
        })
        return ErrPermissionDenied
    }
    
    // ⭐ 5. 速率限制
    if !h.tunnelRateLimiter.Allow(clientID, req.MappingID) {
        utils.Warnf("Client %d exceeded tunnel creation rate limit", clientID)
        return ErrRateLimitExceeded
    }
    
    // ⭐ 6. 并发限制
    activeTunnels := h.getActiveTunnelCount(clientID, req.MappingID)
    if activeTunnels >= h.config.MaxTunnelsPerMapping {
        utils.Warnf("Client %d exceeded max tunnels for mapping %s", clientID, req.MappingID)
        return ErrTunnelLimitExceeded
    }
    
    // ⭐ 7. 验证TunnelID唯一性（防止冲突攻击）
    if h.tunnelExists(req.TunnelID) {
        utils.Warnf("Tunnel ID %s already exists (possible attack)", req.TunnelID)
        return ErrTunnelIDConflict
    }
    
    // ⭐ 8. IP限制（可选，基于映射配置）
    if mapping.Config.IPWhitelist != nil {
        if !isIPInWhitelist(clientIP, mapping.Config.IPWhitelist) {
            return ErrIPNotAllowed
        }
    }
    
    // ⭐ 9. 时间限制（可选，临时映射）
    if mapping.ExpiresAt != nil && time.Now().After(*mapping.ExpiresAt) {
        return ErrMappingExpired
    }
    
    // ⭐ 10. 安全审计（成功）
    h.logSecurityEvent(&SecurityEvent{
        Type:      "tunnel_opened",
        ClientID:  clientID,
        MappingID: req.MappingID,
        TunnelID:  req.TunnelID,
        IPAddress: clientIP,
        Success:   true,
        RiskScore: 1,
    })
    
    return nil
}

// 权限验证
func (h *ServerTunnelHandler) hasPermission(clientID int64, mapping *models.PortMapping) bool {
    // ⭐ 源端客户端权限
    if mapping.SourceClientID == clientID {
        return true
    }
    
    // ⭐ 目标端客户端权限
    if mapping.TargetClientID == clientID {
        return true
    }
    
    // ⭐ 用户级权限（如果客户端属于同一用户）
    if mapping.UserID != "" {
        client, _ := h.cloudControl.GetClient(clientID)
        if client != nil && client.UserID == mapping.UserID {
            return true // 同一用户的客户端可以互相访问
        }
    }
    
    return false
}
```

#### 3.2 隧道速率限制
```go
// TunnelRateLimiter 隧道速率限制器
type TunnelRateLimiter struct {
    limits map[string]*rate.Limiter // "clientID:mappingID" -> limiter
    mu     sync.RWMutex
}

const (
    TunnelsPerSecond = 10  // 每秒最多10个隧道
    BurstSize        = 20  // 突发20个
)

func (l *TunnelRateLimiter) Allow(clientID int64, mappingID string) bool {
    key := fmt.Sprintf("%d:%s", clientID, mappingID)
    
    l.mu.RLock()
    limiter, exists := l.limits[key]
    l.mu.RUnlock()
    
    if !exists {
        l.mu.Lock()
        limiter = rate.NewLimiter(TunnelsPerSecond, BurstSize)
        l.limits[key] = limiter
        l.mu.Unlock()
    }
    
    return limiter.Allow()
}
```

### 🛡️ Layer 4: 会话管理增强

#### 4.1 会话超时
```go
// SessionManager增强
type SessionManager struct {
    // ... 现有字段
    
    // ⭐ 会话管理
    sessionTimeout    time.Duration           // 会话超时时间
    sessionTokens     map[int64]string        // ClientID -> SessionToken
    sessionExpiry     map[int64]time.Time     // ClientID -> 过期时间
    sessionMutex      sync.RWMutex
}

// 验证会话Token
func (s *SessionManager) ValidateSessionToken(clientID int64, token string) error {
    s.sessionMutex.RLock()
    defer s.sessionMutex.RUnlock()
    
    // ⭐ 检查Token是否存在
    storedToken, exists := s.sessionTokens[clientID]
    if !exists {
        return ErrSessionNotFound
    }
    
    // ⭐ 验证Token
    if !hmac.Equal([]byte(storedToken), []byte(token)) {
        return ErrInvalidSessionToken
    }
    
    // ⭐ 检查过期
    expiry, exists := s.sessionExpiry[clientID]
    if exists && time.Now().After(expiry) {
        delete(s.sessionTokens, clientID)
        delete(s.sessionExpiry, clientID)
        return ErrSessionExpired
    }
    
    // ⭐ 续期
    s.sessionExpiry[clientID] = time.Now().Add(s.sessionTimeout)
    
    return nil
}
```

#### 4.2 并发连接限制
```go
// 限制每个客户端的并发连接数
const MaxConnectionsPerClient = 3

func (s *SessionManager) checkConnectionLimit(clientID int64) error {
    s.controlConnLock.RLock()
    count := 0
    for _, conn := range s.controlConnMap {
        if conn.ClientID == clientID {
            count++
        }
    }
    s.controlConnLock.RUnlock()
    
    if count >= MaxConnectionsPerClient {
        return ErrTooManyConnections
    }
    
    return nil
}
```

### 🛡️ Layer 5: IP管理

#### 5.1 IP黑名单/白名单
```go
// IPManager IP管理器
type IPManager struct {
    blacklist  map[string]time.Time // IP -> 解封时间
    whitelist  map[int64][]string   // ClientID -> 允许的IP列表
    mu         sync.RWMutex
}

// 检查IP是否被封禁
func (m *IPManager) IsBlocked(ip string) bool {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    unblockTime, exists := m.blacklist[ip]
    if !exists {
        return false
    }
    
    if time.Now().After(unblockTime) {
        // 自动解封
        m.mu.RUnlock()
        m.mu.Lock()
        delete(m.blacklist, ip)
        m.mu.Unlock()
        return false
    }
    
    return true
}

// 封禁IP
func (m *IPManager) BlockIP(ip string, duration time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.blacklist[ip] = time.Now().Add(duration)
    utils.Warnf("IP %s blocked for %s", ip, duration)
}

// 检查IP是否在白名单
func (m *IPManager) IsWhitelisted(clientID int64, ip string) bool {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    allowedIPs, exists := m.whitelist[clientID]
    if !exists {
        return true // 没有白名单限制
    }
    
    for _, allowedIP := range allowedIPs {
        if matchIP(ip, allowedIP) { // 支持CIDR
            return true
        }
    }
    
    return false
}
```

### 🛡️ Layer 6: 审计与监控

#### 6.1 安全事件日志
```go
// SecurityEvent 安全事件
type SecurityEvent struct {
    Timestamp   time.Time `json:"timestamp"`
    Type        string    `json:"type"`        // "authentication_failed", "tunnel_opened", etc.
    ClientID    int64     `json:"client_id"`
    MappingID   string    `json:"mapping_id,omitempty"`
    TunnelID    string    `json:"tunnel_id,omitempty"`
    IPAddress   string    `json:"ip_address"`
    UserAgent   string    `json:"user_agent,omitempty"`
    Success     bool      `json:"success"`
    ErrorReason string    `json:"error_reason,omitempty"`
    RiskScore   int       `json:"risk_score"`  // 1-10
}

// SecurityLogger 安全日志记录器
type SecurityLogger struct {
    logger *log.Logger
    db     *sql.DB // 存储到数据库
}

func (l *SecurityLogger) Log(event *SecurityEvent) {
    // ⭐ 1. 记录到日志文件
    l.logger.Printf("[SECURITY] %s: %+v", event.Type, event)
    
    // ⭐ 2. 存储到数据库（用于分析）
    l.db.Exec(`
        INSERT INTO security_events 
        (timestamp, type, client_id, ip_address, success, risk_score)
        VALUES (?, ?, ?, ?, ?, ?)`,
        event.Timestamp, event.Type, event.ClientID, event.IPAddress, event.Success, event.RiskScore)
    
    // ⭐ 3. 高风险事件触发告警
    if event.RiskScore >= 8 {
        l.sendAlert(event)
    }
}

func (l *SecurityLogger) sendAlert(event *SecurityEvent) {
    // 发送到监控系统（如Prometheus、Sentry）
    alertMessage := fmt.Sprintf("HIGH RISK: %s from client %d (IP: %s)", 
        event.Type, event.ClientID, event.IPAddress)
    
    // 可以集成：
    // - Email
    // - Slack
    // - PagerDuty
    // - Webhook
}
```

#### 6.2 异常检测
```go
// AnomalyDetector 异常检测器
type AnomalyDetector struct {
    baseline map[int64]*ClientBaseline
    mu       sync.RWMutex
}

type ClientBaseline struct {
    TypicalIPs      []string      // 常用IP
    TypicalGeo      []string      // 常用地理位置
    TypicalHours    []int         // 常用时间段
    AvgConnDuration time.Duration // 平均连接时长
}

// 检测异常行为
func (d *AnomalyDetector) DetectAnomaly(clientID int64, ip string) bool {
    d.mu.RLock()
    baseline, exists := d.baseline[clientID]
    d.mu.RUnlock()
    
    if !exists {
        return false // 新客户端，无基线
    }
    
    anomalyScore := 0
    
    // ⭐ 1. IP异常
    if !contains(baseline.TypicalIPs, ip) {
        anomalyScore += 3
    }
    
    // ⭐ 2. 地理位置异常
    geo := getGeoLocation(ip)
    if !contains(baseline.TypicalGeo, geo) {
        anomalyScore += 4
    }
    
    // ⭐ 3. 时间异常
    currentHour := time.Now().Hour()
    if !contains(baseline.TypicalHours, currentHour) {
        anomalyScore += 2
    }
    
    // ⭐ 异常阈值
    return anomalyScore >= 5
}
```

## 完整认证流程（含安全加固）

### 指令通道连接流程
```
1. TCP/TLS连接建立
   ↓
2. ⭐ 传输层验证：
   - TLS版本 >= 1.2
   - 密码套件安全
   - 客户端证书（可选）
   ↓
3. ⭐ IP检查：
   - 黑名单检查
   - 速率限制
   ↓
4. 发送Handshake请求
   {
       "client_id": 12345678,
       "token": "auth_code",
       "captcha": "xxx" (匿名客户端)
       "mfa_token": "123456" (MFA客户端)
   }
   ↓
5. ⭐ 服务端多层验证：
   5.1 暴力破解检查
   5.2 客户端黑名单检查
   5.3 AuthCode/Token验证
   5.4 IP白名单检查（如有）
   5.5 MFA验证（如需要）
   5.6 重复连接检测
   ↓
6. 生成SessionToken
   ↓
7. 记录TLS指纹
   ↓
8. 更新会话状态
   ↓
9. ⭐ 安全审计
   ↓
10. 返回Handshake响应
    {
        "success": true,
        "session_token": "xxx"
    }
    ↓
11. 连接建立 ✅
```

### 映射通道连接流程
```
1. TCP/TLS连接建立
   ↓
2. ⭐ 传输层验证（同上）
   ↓
3. 发送TunnelOpen请求
   {
       "tunnel_id": "tunnel_xxx",
       "mapping_id": "pmap_xxx",
       "secret_key": "xxx"
   }
   ↓
4. ⭐ 服务端多层验证：
   4.1 Mapping存在性检查
   4.2 SecretKey验证（HMAC比较）
   4.3 ClientID提取（TLS证书或握手）
   4.4 权限验证（⭐ 关键）
   4.5 速率限制
   4.6 并发限制
   4.7 TunnelID唯一性检查
   4.8 IP限制（如有）
   4.9 时间限制（如有）
   ↓
5. ⭐ 安全审计
   ↓
6. 隧道建立 ✅
```

## 安全配置示例

```yaml
security:
  # 传输层
  tls:
    force: true                    # ⭐ 强制TLS
    min_version: "1.2"
    require_client_cert: false     # mTLS（可选）
  
  # 匿名客户端
  anonymous:
    enabled: true
    max_per_ip: 3
    max_per_minute: 100
    require_captcha: false
    default_quota:
      max_mappings: 1
      max_bandwidth: 10           # MB/s
      max_connections: 10
    ttl: 24h
  
  # 认证
  authentication:
    max_failures: 5               # 最多5次失败
    block_duration: 15m           # 封禁15分钟
    session_timeout: 24h
    require_mfa: false            # MFA（可选）
  
  # 速率限制
  rate_limit:
    auth_per_ip_per_minute: 60
    tunnels_per_client_per_second: 10
    global_limit_per_second: 1000
  
  # 连接限制
  connection:
    max_per_client: 3
    max_tunnels_per_mapping: 100
  
  # 审计
  audit:
    enabled: true
    log_file: "/var/log/tunnox/security.log"
    database: true
    alert_on_high_risk: true
```

## 安全检查清单

### 开发阶段
- [ ] 强制TLS >= 1.2
- [ ] 匿名客户端速率限制
- [ ] 暴力破解防护
- [ ] 隧道权限验证
- [ ] SecretKey HMAC比较
- [ ] 会话Token机制
- [ ] TLS指纹绑定
- [ ] 并发连接限制
- [ ] 审计日志记录

### 测试阶段
- [ ] 暴力破解测试（应封禁）
- [ ] 未授权访问测试（应拒绝）
- [ ] 速率限制测试（应限流）
- [ ] 重复连接测试（应踢旧连接）
- [ ] 会话劫持测试（应检测）
- [ ] IP伪造测试（应识别）

### 部署阶段
- [ ] TLS证书配置
- [ ] 密钥安全存储
- [ ] 日志审计启用
- [ ] 监控告警配置
- [ ] 定期安全审计

## 性能影响

| 安全措施 | 延迟增加 | CPU开销 | 值得采用 |
|---------|---------|---------|----------|
| TLS强制 | 5-10ms | 中 | ✅ 必须 |
| IP黑名单检查 | < 1ms | 极低 | ✅ 必须 |
| 暴力破解防护 | < 1ms | 极低 | ✅ 必须 |
| SecretKey HMAC | < 1ms | 极低 | ✅ 必须 |
| 权限验证 | 1-2ms | 低 | ✅ 必须 |
| 速率限制 | < 1ms | 极低 | ✅ 必须 |
| 审计日志 | 异步 | 低 | ✅ 推荐 |
| mTLS | 10-20ms | 中 | ✅ 推荐 |
| MFA | 用户交互 | 低 | ⚠️ 可选 |

**总体影响**：延迟增加 < 30ms，CPU增加 < 10%，**完全可接受**

---

**安全是系统的生命线，必须从设计之初就考虑周全！**


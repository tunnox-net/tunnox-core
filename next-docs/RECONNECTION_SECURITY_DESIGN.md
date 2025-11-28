# 重连与隧道迁移安全设计

## 安全威胁分析

### 🔴 潜在攻击场景

#### 1. 重放攻击（Replay Attack）
```
攻击者截获 TunnelReconnect 请求：
{
    "tunnel_id": "tunnel_xxx",
    "client_id": 12345678,
    "last_seq": 1000
}

攻击者重放此请求 →
  ❌ 劫持隧道，接收敏感数据
  ❌ 冒充合法客户端
```

#### 2. 会话劫持（Session Hijacking）
```
攻击者伪造序列号恢复请求：
{
    "tunnel_id": "tunnel_xxx",
    "reconnect_token": "盗取的token",
    "last_seq": 999  // 伪造，获取之前的数据
}

攻击者获取历史数据 →
  ❌ 数据泄露
  ❌ 破坏数据完整性
```

#### 3. 状态污染（State Poisoning）
```
攻击者在旧服务器关闭前注入恶意状态：
Redis["tunnel_xxx"] = {
    "last_seq": 99999,  // 错误序列号
    "client_id": 攻击者ID
}

正常客户端重连 →
  ❌ 无法恢复（序列号错误）
  ❌ 拒绝服务
```

#### 4. 资源耗尽（Resource Exhaustion）
```
攻击者大量发起重连请求：
for i in range(10000):
    reconnect(fake_tunnel_id, fake_token)

服务器资源耗尽 →
  ❌ 拒绝服务（DoS）
  ❌ 影响正常用户
```

#### 5. 中间人攻击（Man-in-the-Middle）
```
攻击者在重连时插入恶意数据：
Client → [Attacker] → Server

Attacker修改：
- 序列号（跳过某些数据）
- 目标地址（重定向流量）
- 加密密钥（降级攻击）
```

#### 6. 认证绕过（Authentication Bypass）
```
攻击者利用重连机制跳过正常认证：
- 伪造 ReconnectToken
- 利用未清理的旧会话
- 时序攻击（在认证窗口期内）
```

## 安全防护方案

### 🛡️ 多层防御架构

```
Layer 1: 传输层安全（TLS/DTLS）
  ↓
Layer 2: 身份认证（JWT + ReconnectToken）
  ↓
Layer 3: 会话绑定（ClientID + ConnID + Nonce）
  ↓
Layer 4: 时间窗口限制（TTL）
  ↓
Layer 5: 速率限制（Rate Limiting）
  ↓
Layer 6: 状态完整性（HMAC签名）
  ↓
Layer 7: 审计日志（监控异常）
```

### 1. 重连Token机制（核心）

#### 1.1 Token生成
```go
// ReconnectToken 重连凭证（一次性，短时效）
type ReconnectToken struct {
    TokenID      string    // 唯一标识（UUID）
    ClientID     int64     // 客户端ID
    TunnelID     string    // 隧道ID（可选，用于隧道重连）
    NodeID       string    // 签发节点ID
    IssuedAt     time.Time // 签发时间
    ExpiresAt    time.Time // 过期时间（通常5-30秒）
    Nonce        string    // 随机数（防重放）
    Signature    string    // HMAC签名
}

// 生成重连Token
func GenerateReconnectToken(clientID int64, tunnelID string) (*ReconnectToken, error) {
    token := &ReconnectToken{
        TokenID:   uuid.New().String(),
        ClientID:  clientID,
        TunnelID:  tunnelID,
        NodeID:    currentNodeID,
        IssuedAt:  time.Now(),
        ExpiresAt: time.Now().Add(30 * time.Second), // ⭐ 短时效
        Nonce:     generateNonce(32), // ⭐ 防重放
    }
    
    // ⭐ HMAC签名，防止篡改
    token.Signature = signToken(token, serverSecretKey)
    
    // ⭐ 存储到Redis（一次性使用后删除）
    storeReconnectToken(token)
    
    return token, nil
}

// HMAC签名
func signToken(token *ReconnectToken, secretKey []byte) string {
    data := fmt.Sprintf("%s:%d:%s:%s:%d:%s",
        token.TokenID, token.ClientID, token.TunnelID,
        token.NodeID, token.IssuedAt.Unix(), token.Nonce)
    
    mac := hmac.New(sha256.New, secretKey)
    mac.Write([]byte(data))
    return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
```

#### 1.2 Token验证
```go
// 验证重连Token
func ValidateReconnectToken(token *ReconnectToken) error {
    // 1. ⭐ 验证签名（防篡改）
    expectedSig := signToken(token, serverSecretKey)
    if !hmac.Equal([]byte(token.Signature), []byte(expectedSig)) {
        return ErrInvalidSignature
    }
    
    // 2. ⭐ 检查过期（防重放）
    if time.Now().After(token.ExpiresAt) {
        return ErrTokenExpired
    }
    
    // 3. ⭐ 检查Nonce（防重放）
    if !checkAndConsumeNonce(token.Nonce) {
        return ErrNonceAlreadyUsed // 已被使用过
    }
    
    // 4. ⭐ 验证Redis中的Token（一次性使用）
    storedToken, err := getReconnectToken(token.TokenID)
    if err != nil {
        return ErrTokenNotFound // Token不存在或已使用
    }
    
    // 5. ⭐ 对比Token内容
    if storedToken.ClientID != token.ClientID ||
       storedToken.TunnelID != token.TunnelID {
        return ErrTokenMismatch
    }
    
    // 6. ⭐ 删除Token（确保一次性使用）
    deleteReconnectToken(token.TokenID)
    
    return nil
}
```

#### 1.3 优雅关闭时分发Token
```go
// ServerShutdown命令（增强版）
type ServerShutdownCommand struct {
    Reason           string            `json:"reason"`         // "rolling_update"
    GracePeriod      int               `json:"grace_period"`   // 10秒
    ReconnectToken   *ReconnectToken   `json:"reconnect_token"` // ⭐ 一次性重连凭证
}

// 优雅关闭流程
func (s *SessionManager) GracefulShutdown() {
    // 1. 为每个控制连接生成 ReconnectToken
    s.controlConnLock.RLock()
    for _, conn := range s.controlConnMap {
        token, _ := GenerateReconnectToken(conn.ClientID, "")
        
        // 2. 发送 ServerShutdown 命令（携带Token）
        s.sendServerShutdown(conn, &ServerShutdownCommand{
            Reason:         "rolling_update",
            GracePeriod:    10,
            ReconnectToken: token, // ⭐ 客户端收到后用于重连
        })
    }
    s.controlConnLock.RUnlock()
    
    // 3. 等待客户端迁移
    s.WaitForTunnelsToComplete(10 * time.Second)
    
    // 4. 关闭剩余连接
    s.closeAllConnections()
}
```

### 2. 会话绑定（Session Binding）

#### 2.1 多因子绑定
```go
// SessionIdentity 会话身份（多因子）
type SessionIdentity struct {
    ClientID     int64     // 客户端ID
    ConnID       string    // 连接ID（每次连接不同）
    IPAddress    string    // 客户端IP（可选，防IP变更）
    TLSFingerprint string  // TLS指纹（防中间人）
    UserAgent    string    // 客户端版本（可选）
}

// 验证会话身份
func ValidateSessionIdentity(claimed, stored *SessionIdentity) error {
    // ⭐ 必须匹配ClientID
    if claimed.ClientID != stored.ClientID {
        return ErrClientIDMismatch
    }
    
    // ⭐ 可选：验证IP地址（考虑移动网络IP变更）
    if config.RequireIPMatch && claimed.IPAddress != stored.IPAddress {
        utils.Warnf("Client %d IP changed: %s -> %s",
            claimed.ClientID, stored.IPAddress, claimed.IPAddress)
        // 可以允许，但记录异常
    }
    
    // ⭐ 必须：验证TLS指纹（防中间人）
    if claimed.TLSFingerprint != stored.TLSFingerprint {
        return ErrTLSFingerprintMismatch // 严重安全问题
    }
    
    return nil
}
```

#### 2.2 TLS客户端证书（推荐）
```go
// 使用mTLS（双向TLS）增强安全性
type TLSConfig struct {
    // 服务器证书
    ServerCert string
    ServerKey  string
    
    // ⭐ 客户端证书（可选但推荐）
    ClientCA   string // 信任的客户端CA
    RequireClientCert bool // 是否强制客户端证书
}

// 在TLS握手时验证客户端证书
func (s *Server) verifyClientCert(rawCerts [][]byte) error {
    // 从证书提取ClientID
    clientID := extractClientIDFromCert(rawCerts[0])
    
    // 验证证书是否被吊销
    if isCertRevoked(clientID) {
        return ErrCertRevoked
    }
    
    return nil
}
```

### 3. 隧道状态完整性

#### 3.1 状态签名
```go
// TunnelState 隧道状态（带签名）
type TunnelState struct {
    TunnelID       string    `json:"tunnel_id"`
    MappingID      string    `json:"mapping_id"`
    SourceClientID int64     `json:"source_client_id"`
    TargetClientID int64     `json:"target_client_id"`
    LastSeqNum     uint64    `json:"last_seq_num"`
    LastAckNum     uint64    `json:"last_ack_num"`
    UpdatedAt      time.Time `json:"updated_at"`
    Signature      string    `json:"signature"` // ⭐ HMAC签名
}

// 保存状态时签名
func SaveTunnelState(state *TunnelState) error {
    // ⭐ 计算签名
    state.Signature = signTunnelState(state, serverSecretKey)
    
    // 存储到Redis
    return redis.Set(ctx, "tunnel:state:"+state.TunnelID, state, 5*time.Minute)
}

// 加载状态时验证
func LoadTunnelState(tunnelID string) (*TunnelState, error) {
    state := &TunnelState{}
    err := redis.Get(ctx, "tunnel:state:"+tunnelID, state)
    if err != nil {
        return nil, err
    }
    
    // ⭐ 验证签名
    expectedSig := signTunnelState(state, serverSecretKey)
    if !hmac.Equal([]byte(state.Signature), []byte(expectedSig)) {
        utils.Errorf("Tunnel state signature mismatch for %s", tunnelID)
        return nil, ErrInvalidStateSignature // ⭐ 状态被篡改
    }
    
    return state, nil
}
```

#### 3.2 序列号范围验证
```go
// 验证序列号合理性（防止攻击者伪造）
func ValidateSeqNum(claimed, stored uint64) error {
    const maxJump = 10000 // 最大允许跳跃（可配置）
    
    // ⭐ 序列号不能倒退
    if claimed < stored {
        return ErrSeqNumRewind
    }
    
    // ⭐ 序列号不能跳跃太大（防止攻击者伪造）
    if claimed - stored > maxJump {
        utils.Errorf("Seq num jump too large: %d -> %d", stored, claimed)
        return ErrSeqNumJumpTooLarge
    }
    
    return nil
}
```

### 4. 时间窗口限制

#### 4.1 重连时间窗口
```go
const (
    // ⭐ 重连必须在服务器关闭后的时间窗口内
    ReconnectWindowAfterShutdown = 30 * time.Second
    
    // ⭐ 状态保留时间（超过则清理）
    StateRetentionTime = 5 * time.Minute
)

// 验证重连时机
func ValidateReconnectTiming(shutdownTime, reconnectTime time.Time) error {
    elapsed := reconnectTime.Sub(shutdownTime)
    
    // ⭐ 太快（可能是预测攻击）
    if elapsed < 0 {
        return ErrReconnectTooEarly
    }
    
    // ⭐ 太慢（状态已清理）
    if elapsed > ReconnectWindowAfterShutdown {
        return ErrReconnectTooLate
    }
    
    return nil
}
```

#### 4.2 状态自动清理
```go
// 定期清理过期状态（防止状态泄露）
func (s *SessionManager) cleanupExpiredStates() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            // ⭐ 清理超过5分钟的隧道状态
            expiredStates := redis.Scan("tunnel:state:*")
            for _, key := range expiredStates {
                state, _ := redis.Get(key)
                if time.Since(state.UpdatedAt) > StateRetentionTime {
                    redis.Delete(key)
                    utils.Infof("Cleaned up expired tunnel state: %s", key)
                }
            }
            
            // ⭐ 清理过期的重连Token
            expiredTokens := redis.Scan("reconnect:token:*")
            for _, key := range expiredTokens {
                token, _ := redis.Get(key)
                if time.Since(token.ExpiresAt) > 0 {
                    redis.Delete(key)
                }
            }
            
        case <-s.Ctx().Done():
            return
        }
    }
}
```

### 5. 速率限制（Rate Limiting）

#### 5.1 重连频率限制
```go
// RateLimiter 速率限制器
type ReconnectRateLimiter struct {
    limits map[int64]*ClientLimit // ClientID -> Limit
    mu     sync.RWMutex
}

type ClientLimit struct {
    Count      int       // 重连次数
    WindowStart time.Time // 时间窗口开始
}

const (
    MaxReconnectsPerMinute = 10 // ⭐ 每分钟最多10次重连
)

// 检查是否允许重连
func (r *ReconnectRateLimiter) AllowReconnect(clientID int64) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    limit, exists := r.limits[clientID]
    if !exists {
        // 首次重连
        r.limits[clientID] = &ClientLimit{
            Count:      1,
            WindowStart: time.Now(),
        }
        return nil
    }
    
    // ⭐ 检查时间窗口
    if time.Since(limit.WindowStart) > 1*time.Minute {
        // 重置窗口
        limit.Count = 1
        limit.WindowStart = time.Now()
        return nil
    }
    
    // ⭐ 检查频率
    if limit.Count >= MaxReconnectsPerMinute {
        utils.Warnf("Client %d exceeded reconnect rate limit", clientID)
        return ErrRateLimitExceeded
    }
    
    limit.Count++
    return nil
}
```

#### 5.2 全局速率限制
```go
// 防止DDoS攻击
type GlobalRateLimiter struct {
    tokenBucket *rate.Limiter
}

func NewGlobalRateLimiter() *GlobalRateLimiter {
    // ⭐ 每秒最多100个重连请求
    return &GlobalRateLimiter{
        tokenBucket: rate.NewLimiter(100, 200), // 100 req/s, burst 200
    }
}

func (g *GlobalRateLimiter) Allow() bool {
    return g.tokenBucket.Allow()
}
```

### 6. 传输层安全（TLS/DTLS）

#### 6.1 强制TLS
```go
// 配置TLS（强制）
type SecurityConfig struct {
    // ⭐ 禁用非TLS连接
    AllowPlaintext bool // 默认false（生产环境必须false）
    
    // ⭐ TLS版本要求
    MinTLSVersion uint16 // tls.VersionTLS12（最低1.2）
    
    // ⭐ 密码套件（禁用弱密码）
    CipherSuites []uint16
}

// TLS配置
func createTLSConfig(cfg *SecurityConfig) *tls.Config {
    return &tls.Config{
        MinVersion: tls.VersionTLS12, // ⭐ 禁用TLS 1.0/1.1
        CipherSuites: []uint16{
            tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
            // ⭐ 禁用弱密码（如RC4, DES等）
        },
        PreferServerCipherSuites: true,
    }
}
```

#### 6.2 QUIC集成（推荐）
```
QUIC的内置安全特性：
- ✅ 强制TLS 1.3
- ✅ 连接ID（支持连接迁移）
- ✅ 防重放（内置nonce）
- ✅ 前向保密（每个连接独立密钥）
```

### 7. 审计与监控

#### 7.1 安全事件日志
```go
// SecurityEvent 安全事件
type SecurityEvent struct {
    Timestamp   time.Time `json:"timestamp"`
    EventType   string    `json:"event_type"` // "reconnect", "token_invalid", etc.
    ClientID    int64     `json:"client_id"`
    IPAddress   string    `json:"ip_address"`
    Success     bool      `json:"success"`
    ErrorReason string    `json:"error_reason,omitempty"`
    RiskScore   int       `json:"risk_score"` // 1-10
}

// 记录安全事件
func LogSecurityEvent(event *SecurityEvent) {
    // ⭐ 记录到专门的安全日志
    securityLogger.Info(event)
    
    // ⭐ 高风险事件触发告警
    if event.RiskScore >= 7 {
        alertSystem.Trigger("high_risk_security_event", event)
    }
    
    // ⭐ 存储到数据库（用于分析）
    db.InsertSecurityEvent(event)
}
```

#### 7.2 异常检测
```go
// 检测异常重连模式
func DetectAnomalousReconnect(clientID int64) bool {
    // ⭐ 检查重连频率
    recentReconnects := getReconnectHistory(clientID, 1*time.Hour)
    if len(recentReconnects) > 100 {
        return true // 异常高频重连
    }
    
    // ⭐ 检查IP变化频率
    ips := extractIPs(recentReconnects)
    if len(ips) > 10 {
        return true // IP频繁变化（可能被盗用）
    }
    
    // ⭐ 检查时间模式
    if hasRegularPattern(recentReconnects) {
        return true // 机器人行为
    }
    
    return false
}
```

## 完整重连流程（含安全验证）

### 控制连接重连流程
```
1. Server发送ServerShutdown（携带ReconnectToken）
   ↓
2. Client收到通知，保存ReconnectToken
   ↓
3. Client断开连接（或被动断开）
   ↓
4. Client立即重连（携带ReconnectToken）
   {
       "command_type": "Reconnect",
       "client_id": 12345678,
       "reconnect_token": {
           "token_id": "uuid",
           "signature": "hmac_sig",
           ...
       }
   }
   ↓
5. Server验证（多层）：
   5.1 ✅ TLS连接验证
   5.2 ✅ Token签名验证（HMAC）
   5.3 ✅ Token未过期（30秒内）
   5.4 ✅ Nonce未被使用
   5.5 ✅ 速率限制通过
   5.6 ✅ 客户端身份匹配（TLS指纹）
   ↓
6. Server删除Token（一次性使用）
   ↓
7. Server恢复会话：
   - 更新ClientRuntimeState
   - 推送配置（如需要）
   ↓
8. 重连成功 ✅
```

### 隧道重连流程
```
1. 隧道传输中断
   ↓
2. Client检测断开
   ↓
3. Client发起TunnelReconnect：
   {
       "tunnel_id": "tunnel_xxx",
       "reconnect_token": {...},
       "last_sent_seq": 1000,
       "last_ack_seq": 999
   }
   ↓
4. Server验证（严格）：
   4.1 ✅ ReconnectToken验证（同上）
   4.2 ✅ TunnelState签名验证
   4.3 ✅ 序列号范围验证（防伪造）
   4.4 ✅ Client拥有此Tunnel权限
   ↓
5. Server加载TunnelState（从Redis）
   ↓
6. Server对比序列号：
   if client.lastSeq == server.lastSeq:
       ✅ 无数据丢失，继续传输
   else:
       ⚠️ 重传差异数据
   ↓
7. 隧道恢复 ✅
```

## 安全配置建议

### 生产环境配置
```yaml
security:
  # ⭐ 传输层
  tls:
    enabled: true          # 必须启用
    min_version: 1.2       # 最低TLS 1.2
    require_client_cert: true  # 推荐启用mTLS
  
  # ⭐ 重连Token
  reconnect_token:
    ttl: 30s              # 短时效（30秒）
    max_uses: 1           # 一次性使用
    hmac_algorithm: sha256
  
  # ⭐ 速率限制
  rate_limit:
    max_reconnects_per_minute: 10
    global_limit_per_second: 100
  
  # ⭐ 状态管理
  state:
    retention_time: 5m    # 状态保留5分钟
    signature_required: true
  
  # ⭐ 审计
  audit:
    log_all_reconnects: true
    alert_on_anomaly: true
```

## 安全检查清单

### 开发阶段
- [ ] ReconnectToken使用HMAC签名
- [ ] Token一次性使用（用后即焚）
- [ ] Nonce防重放机制
- [ ] 序列号范围验证
- [ ] TLS最低版本1.2
- [ ] 禁用弱密码套件
- [ ] 实现速率限制
- [ ] 状态签名验证
- [ ] 时间窗口限制
- [ ] 审计日志记录

### 测试阶段
- [ ] 重放攻击测试（应失败）
- [ ] Token过期测试（应拒绝）
- [ ] 伪造序列号测试（应检测）
- [ ] 速率限制测试（应限流）
- [ ] 并发重连测试（性能）
- [ ] 中间人攻击测试（TLS）
- [ ] 状态污染测试（签名验证）

### 部署阶段
- [ ] 证书正确配置
- [ ] 密钥安全存储（不在代码中）
- [ ] 监控告警配置
- [ ] 日志审计启用
- [ ] 定期安全审计

## 性能影响评估

| 安全措施 | CPU开销 | 延迟增加 | 内存开销 | 值得采用 |
|---------|---------|---------|---------|----------|
| **TLS/DTLS** | 中 | 5-10ms | 低 | ✅ 必须 |
| **HMAC签名** | 极低 | < 1ms | 极低 | ✅ 必须 |
| **Nonce检查** | 极低 | < 1ms | 低（Redis） | ✅ 必须 |
| **速率限制** | 极低 | < 1ms | 低 | ✅ 推荐 |
| **状态签名** | 极低 | < 1ms | 极低 | ✅ 推荐 |
| **mTLS（客户端证书）** | 中 | 10-20ms | 低 | ✅ 推荐 |
| **审计日志** | 低 | 异步，无影响 | 中 | ✅ 推荐 |

**总体影响**：延迟增加 < 20ms，CPU增加 < 5%，**完全可接受**

## 总结

### 关键安全措施（必须实施）

1. ✅ **ReconnectToken**：一次性、短时效、HMAC签名
2. ✅ **TLS强制**：禁用明文传输
3. ✅ **Nonce防重放**：Redis存储，用后即焚
4. ✅ **速率限制**：防DDoS
5. ✅ **状态签名**：防篡改

### 推荐安全措施（生产环境）

6. ✅ **mTLS**：客户端证书认证
7. ✅ **审计日志**：全面记录
8. ✅ **异常检测**：智能告警

### 对现有架构的影响

- ✅ **最小化改动**：主要新增验证逻辑
- ✅ **向后兼容**：非重连场景不受影响
- ✅ **性能友好**：延迟增加 < 20ms

---

**安全是第一要务，宁可牺牲一点性能，也要确保系统不被攻击者利用！**


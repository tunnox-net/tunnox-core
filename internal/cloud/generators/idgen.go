package generators

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"tunnox-core/internal/utils"
)

// 错误定义
var (
	ErrIDExhausted = errors.New("ID exhausted")
)

// 常量定义
const (
	// ID生成相关常量
	ClientIDMin     = int64(10000000)
	ClientIDMax     = int64(99999999)
	ClientIDLength  = 8
	AuthCodeLength  = 6
	SecretKeyLength = 32
	NodeIDLength    = 16
	UserIDLength    = 16
	MappingIDLength = 12
	MaxAttempts     = 100
)

// IDGenerator ID生成器
type IDGenerator struct {
	usedIDs        map[int64]bool
	usedNodeIDs    map[string]bool
	usedUserIDs    map[string]bool
	usedMappingIDs map[string]bool
	mu             sync.RWMutex
}

// NewIDGenerator 创建新的ID生成器
func NewIDGenerator() *IDGenerator {
	return &IDGenerator{
		usedIDs:        make(map[int64]bool),
		usedNodeIDs:    make(map[string]bool),
		usedUserIDs:    make(map[string]bool),
		usedMappingIDs: make(map[string]bool),
	}
}

// GenerateClientID 生成客户端ID（8位大于10000000的随机整数）
func (g *IDGenerator) GenerateClientID() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		randomInt, err := utils.GenerateRandomInt64(ClientIDMin, ClientIDMax)
		if err != nil {
			return 0, err
		}

		// 检查是否已使用
		if !g.usedIDs[randomInt] {
			g.usedIDs[randomInt] = true
			return randomInt, nil
		}
	}

	return 0, ErrIDExhausted
}

// GenerateAuthCode 生成认证码（类似TeamViewer的6位数字）
func (g *IDGenerator) GenerateAuthCode() (string, error) {
	return utils.GenerateRandomDigits(AuthCodeLength)
}

// GenerateSecretKey 生成密钥（32位随机字符串）
func (g *IDGenerator) GenerateSecretKey() (string, error) {
	return utils.GenerateRandomString(SecretKeyLength)
}

// GenerateNodeID 生成节点ID
func (g *IDGenerator) GenerateNodeID() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		nodeID, err := utils.GenerateRandomString(NodeIDLength)
		if err != nil {
			return "", err
		}

		// 检查是否已使用
		if !g.usedNodeIDs[nodeID] {
			g.usedNodeIDs[nodeID] = true
			return nodeID, nil
		}
	}

	return "", ErrIDExhausted
}

// GenerateUserID 生成用户ID
func (g *IDGenerator) GenerateUserID() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		userID, err := utils.GenerateRandomString(UserIDLength)
		if err != nil {
			return "", err
		}

		// 检查是否已使用
		if !g.usedUserIDs[userID] {
			g.usedUserIDs[userID] = true
			return userID, nil
		}
	}

	return "", ErrIDExhausted
}

// GenerateMappingID 生成端口映射ID
func (g *IDGenerator) GenerateMappingID() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		mappingID, err := utils.GenerateRandomString(MappingIDLength)
		if err != nil {
			return "", err
		}

		// 检查是否已使用
		if !g.usedMappingIDs[mappingID] {
			g.usedMappingIDs[mappingID] = true
			return mappingID, nil
		}
	}

	return "", ErrIDExhausted
}

// ReleaseClientID 释放客户端ID
func (g *IDGenerator) ReleaseClientID(clientID int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.usedIDs, clientID)
}

// IsClientIDUsed 检查客户端ID是否已使用
func (g *IDGenerator) IsClientIDUsed(clientID int64) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.usedIDs[clientID]
}

// ReleaseNodeID 释放节点ID
func (g *IDGenerator) ReleaseNodeID(nodeID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.usedNodeIDs, nodeID)
}

// IsNodeIDUsed 检查节点ID是否已使用
func (g *IDGenerator) IsNodeIDUsed(nodeID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.usedNodeIDs[nodeID]
}

// ReleaseUserID 释放用户ID
func (g *IDGenerator) ReleaseUserID(userID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.usedUserIDs, userID)
}

// IsUserIDUsed 检查用户ID是否已使用
func (g *IDGenerator) IsUserIDUsed(userID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.usedUserIDs[userID]
}

// ReleaseMappingID 释放端口映射ID
func (g *IDGenerator) ReleaseMappingID(mappingID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.usedMappingIDs, mappingID)
}

// IsMappingIDUsed 检查端口映射ID是否已使用
func (g *IDGenerator) IsMappingIDUsed(mappingID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.usedMappingIDs[mappingID]
}

// GetUsedCount 获取已使用的ID数量
func (g *IDGenerator) GetUsedCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.usedIDs) + len(g.usedNodeIDs) + len(g.usedUserIDs) + len(g.usedMappingIDs)
}

// ConnectionIDGenerator 连接ID生成器
type ConnectionIDGenerator struct {
	counter int64
	mu      sync.Mutex
}

func NewConnectionIDGenerator() *ConnectionIDGenerator {
	return &ConnectionIDGenerator{}
}

func (g *ConnectionIDGenerator) GenerateID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counter++
	return fmt.Sprintf("conn_%d_%d", time.Now().Unix(), g.counter)
}

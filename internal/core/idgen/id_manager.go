package idgen

import (
	"context"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/utils"
)

// IDManager 统一ID管理器
type IDManager struct {
	storage storage.Storage

	// 不同类型的专门生成器实例
	clientIDGen              IDGenerator[int64]
	nodeIDGen                IDGenerator[string]
	connectionIDGen          IDGenerator[string]
	portMappingIDGen         IDGenerator[string]
	portMappingInstanceIDGen IDGenerator[string]
	userIDGen                IDGenerator[string]
	tunnelIDGen              IDGenerator[string]

	dispose.Dispose
}

// NewIDManager 创建ID管理器
func NewIDManager(storage storage.Storage, parentCtx context.Context) *IDManager {
	manager := &IDManager{
		storage: storage,
	}

	// 初始化各种ID生成器
	// ClientID 使用 int64 类型，生成完全随机的 8 位数字
	manager.clientIDGen = NewStorageIDGenerator[int64](storage, "", "tunnox:id:used:client", parentCtx)

	// 其他 ID 使用 string 类型，生成带前缀的随机字符串
	manager.nodeIDGen = NewStorageIDGenerator[string](storage, PrefixNodeID, "tunnox:id:used:node", parentCtx)
	manager.connectionIDGen = NewStorageIDGenerator[string](storage, PrefixConnectionID, "tunnox:id:used:conn", parentCtx)
	manager.portMappingIDGen = NewStorageIDGenerator[string](storage, PrefixPortMappingID, "tunnox:id:used:pmap", parentCtx)
	manager.portMappingInstanceIDGen = NewStorageIDGenerator[string](storage, PrefixPortMappingInstanceID, "tunnox:id:used:pmi", parentCtx)
	manager.userIDGen = NewStorageIDGenerator[string](storage, PrefixUserID, "tunnox:id:used:user", parentCtx)
	manager.tunnelIDGen = NewStorageIDGenerator[string](storage, PrefixTunnelID, "tunnox:id:used:tunnel", parentCtx)

	manager.SetCtx(parentCtx, manager.onClose)
	return manager
}

// onClose 资源清理回调
func (m *IDManager) onClose() error {
	corelog.Infof("Cleaning up ID manager resources...")

	// 关闭所有生成器
	if m.clientIDGen != nil {
		err := m.clientIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed client ID generator")
	}

	if m.nodeIDGen != nil {
		err := m.nodeIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed node ID generator")
	}

	if m.connectionIDGen != nil {
		err := m.connectionIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed connection ID generator")
	}

	if m.portMappingIDGen != nil {

		err := m.portMappingIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed port mapping ID generator")
	}

	if m.portMappingInstanceIDGen != nil {
		err := m.portMappingInstanceIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed port mapping instance ID generator")
	}

	if m.userIDGen != nil {
		err := m.userIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed user ID generator")
	}

	if m.tunnelIDGen != nil {
		err := m.tunnelIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed tunnel ID generator")
	}

	corelog.Infof("ID manager resources cleanup completed")
	return nil
}

// 便捷方法
func (m *IDManager) GenerateClientID() (int64, error) {
	return m.clientIDGen.Generate()
}

func (m *IDManager) GenerateNodeID() (string, error) {
	return m.nodeIDGen.Generate()
}

func (m *IDManager) GenerateUserID() (string, error) {
	return m.userIDGen.Generate()
}

func (m *IDManager) GeneratePortMappingID() (string, error) {
	return m.portMappingIDGen.Generate()
}

func (m *IDManager) GeneratePortMappingInstanceID() (string, error) {
	return m.portMappingInstanceIDGen.Generate()
}

func (m *IDManager) GenerateConnectionID() (string, error) {
	return m.connectionIDGen.Generate()
}

func (m *IDManager) GenerateTunnelID() (string, error) {
	return m.tunnelIDGen.Generate()
}

func (m *IDManager) ReleaseClientID(id int64) error {
	return m.clientIDGen.Release(id)
}

func (m *IDManager) ReleaseNodeID(id string) error {
	return m.nodeIDGen.Release(id)
}

func (m *IDManager) ReleaseUserID(id string) error {
	return m.userIDGen.Release(id)
}

func (m *IDManager) ReleasePortMappingID(id string) error {
	return m.portMappingIDGen.Release(id)
}

func (m *IDManager) ReleasePortMappingInstanceID(id string) error {
	return m.portMappingInstanceIDGen.Release(id)
}

func (m *IDManager) ReleaseConnectionID(id string) error {
	return m.connectionIDGen.Release(id)
}

func (m *IDManager) ReleaseTunnelID(id string) error {
	return m.tunnelIDGen.Release(id)
}

func (m *IDManager) IsClientIDUsed(id int64) (bool, error) {
	return m.clientIDGen.IsUsed(id)
}

func (m *IDManager) IsNodeIDUsed(id string) (bool, error) {
	return m.nodeIDGen.IsUsed(id)
}

func (m *IDManager) IsUserIDUsed(id string) (bool, error) {
	return m.userIDGen.IsUsed(id)
}

func (m *IDManager) IsPortMappingIDUsed(id string) (bool, error) {
	return m.portMappingIDGen.IsUsed(id)
}

func (m *IDManager) IsPortMappingInstanceIDUsed(id string) (bool, error) {
	return m.portMappingInstanceIDGen.IsUsed(id)
}

func (m *IDManager) IsConnectionIDUsed(id string) (bool, error) {
	return m.connectionIDGen.IsUsed(id)
}

func (m *IDManager) IsTunnelIDUsed(id string) (bool, error) {
	return m.tunnelIDGen.IsUsed(id)
}

// GenerateAuthCode 生成认证码
func (m *IDManager) GenerateAuthCode() (string, error) {
	return utils.GenerateRandomDigits(6)
}

// GenerateSecretKey 生成密钥
func (m *IDManager) GenerateSecretKey() (string, error) {
	return utils.GenerateRandomString(32)
}

// Close 关闭ID管理器
func (m *IDManager) Close() error {
	m.Dispose.Close()
	return nil
}

// GenerateUniqueID 通用ID生成重试函数
// 用于生成唯一ID，自动处理重试和冲突检查
func (m *IDManager) GenerateUniqueID(
	generateFunc func() (int64, error),
	checkFunc func(int64) (bool, error),
	releaseFunc func(int64) error,
	idType string,
) (int64, error) {
	for attempts := 0; attempts < MaxAttempts; attempts++ {
		generatedID, err := generateFunc()
		if err != nil {
			return 0, coreerrors.Wrapf(err, coreerrors.CodeInternal, "generate %s ID failed", idType)
		}

		// 检查是否已存在
		exists, err := checkFunc(generatedID)
		if err != nil {
			// 如果检查失败，假设不存在，使用这个ID
			return generatedID, nil
		}

		if !exists {
			// ID不存在，可以使用
			return generatedID, nil
		}

		// ID已存在，释放并重试（忽略释放错误，继续尝试生成新ID）
		_ = releaseFunc(generatedID)
		continue
	}

	return 0, coreerrors.Newf(coreerrors.CodeResourceExhausted, "failed to generate unique %s ID after %d attempts", idType, MaxAttempts)
}

// GenerateUniqueClientID 生成唯一客户端ID
func (m *IDManager) GenerateUniqueClientID(checkFunc func(int64) (bool, error)) (int64, error) {
	return m.GenerateUniqueID(
		m.GenerateClientID,
		checkFunc,
		m.ReleaseClientID,
		"client",
	)
}

// GenerateUniquePortMappingID 生成唯一端口映射ID
func (m *IDManager) GenerateUniquePortMappingID(checkFunc func(string) (bool, error)) (string, error) {
	for attempts := 0; attempts < MaxAttempts; attempts++ {
		generatedID, err := m.GeneratePortMappingID()
		if err != nil {
			return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "generate port mapping ID failed")
		}

		// 检查是否已存在
		exists, err := checkFunc(generatedID)
		if err != nil {
			// 如果检查失败，假设不存在，使用这个ID
			return generatedID, nil
		}

		if !exists {
			// ID不存在，可以使用
			return generatedID, nil
		}

		// ID已存在，释放并重试（忽略释放错误，继续尝试生成新ID）
		_ = m.ReleasePortMappingID(generatedID)
		continue
	}

	return "", coreerrors.Newf(coreerrors.CodeResourceExhausted, "failed to generate unique port mapping ID after %d attempts", MaxAttempts)
}

// GenerateUniqueNodeID 生成唯一节点ID
func (m *IDManager) GenerateUniqueNodeID(checkFunc func(string) (bool, error)) (string, error) {
	for attempts := 0; attempts < MaxAttempts; attempts++ {
		generatedID, err := m.GenerateNodeID()
		if err != nil {
			return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "generate node ID failed")
		}

		// 检查是否已存在
		exists, err := checkFunc(generatedID)
		if err != nil {
			// 如果检查失败，假设不存在，使用这个ID
			return generatedID, nil
		}

		if !exists {
			// ID不存在，可以使用
			return generatedID, nil
		}

		// ID已存在，释放并重试（忽略释放错误，继续尝试生成新ID）
		_ = m.ReleaseNodeID(generatedID)
		continue
	}

	return "", coreerrors.Newf(coreerrors.CodeResourceExhausted, "failed to generate unique node ID after %d attempts", MaxAttempts)
}

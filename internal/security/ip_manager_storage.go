package security

import (
	"encoding/json"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// Storage keys
const (
	keyPrefixBlacklist = "tunnox:security:ip:blacklist:"
	keyPrefixWhitelist = "tunnox:security:ip:whitelist:"
	keyIndexBlacklist  = "tunnox:security:ip:blacklist:index"
	keyIndexWhitelist  = "tunnox:security:ip:whitelist:index"
)

// loadFromStorage 从Storage加载黑白名单
func (m *IPManager) loadFromStorage() error {
	if m.storage == nil {
		return coreerrors.New(coreerrors.CodeStorageError, "storage not available")
	}

	// 加载黑名单
	if err := m.loadListFromStorage(IPTypeBlacklist); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to load blacklist")
	}

	// 加载白名单
	if err := m.loadListFromStorage(IPTypeWhitelist); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to load whitelist")
	}

	corelog.Infof("IPManager: loaded %d blacklist and %d whitelist entries from storage",
		len(m.blacklist), len(m.whitelist))

	return nil
}

// loadListFromStorage 从Storage加载指定类型的列表
func (m *IPManager) loadListFromStorage(ipType IPType) error {
	var indexKey string
	var keyPrefix string
	var targetList map[string]*IPRecord

	switch ipType {
	case IPTypeBlacklist:
		indexKey = keyIndexBlacklist
		keyPrefix = keyPrefixBlacklist
		targetList = m.blacklist
	case IPTypeWhitelist:
		indexKey = keyIndexWhitelist
		keyPrefix = keyPrefixWhitelist
		targetList = m.whitelist
	default:
		return coreerrors.New(coreerrors.CodeInvalidParam, "invalid IP type: "+string(ipType))
	}

	// 获取IP列表
	listStore, ok := m.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeStorageError, "storage does not support list operations")
	}
	ips, err := listStore.GetList(indexKey)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return nil // 没有数据，不是错误
		}
		return err
	}

	// 加载每个IP的记录
	for _, ipInterface := range ips {
		ipStr, ok := ipInterface.(string)
		if !ok {
			continue
		}

		key := keyPrefix + ipStr
		data, err := m.storage.Get(key)
		if err != nil {
			continue
		}

		record := &IPRecord{}
		dataBytes, ok := data.([]byte)
		if !ok {
			if dataStr, ok := data.(string); ok {
				dataBytes = []byte(dataStr)
			} else {
				continue
			}
		}
		if err := json.Unmarshal(dataBytes, record); err != nil {
			continue
		}

		targetList[ipStr] = record
	}

	return nil
}

// saveToStorage 保存到Storage
func (m *IPManager) saveToStorage(ipType IPType, ip string, record *IPRecord) error {
	if m.storage == nil {
		return coreerrors.New(coreerrors.CodeStorageError, "storage not available")
	}

	var indexKey string
	var keyPrefix string

	switch ipType {
	case IPTypeBlacklist:
		indexKey = keyIndexBlacklist
		keyPrefix = keyPrefixBlacklist
	case IPTypeWhitelist:
		indexKey = keyIndexWhitelist
		keyPrefix = keyPrefixWhitelist
	default:
		return coreerrors.New(coreerrors.CodeStorageError, "invalid IP type: "+string(ipType))
	}

	// 保存记录
	key := keyPrefix + ip
	data, err := json.Marshal(record)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to encode record")
	}

	var ttl time.Duration
	if !record.ExpiresAt.IsZero() {
		ttl = time.Until(record.ExpiresAt)
		if ttl < 0 {
			ttl = 0
		}
	}

	if err := m.storage.Set(key, data, ttl); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to save record")
	}

	// 添加到索引
	listStore, ok := m.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeStorageError, "storage does not support list operations")
	}
	if err := listStore.AppendToList(indexKey, ip); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to add to index")
	}

	return nil
}

// removeFromStorage 从Storage删除
func (m *IPManager) removeFromStorage(ipType IPType, ip string) error {
	if m.storage == nil {
		return coreerrors.New(coreerrors.CodeStorageError, "storage not available")
	}

	var indexKey string
	var keyPrefix string

	switch ipType {
	case IPTypeBlacklist:
		indexKey = keyIndexBlacklist
		keyPrefix = keyPrefixBlacklist
	case IPTypeWhitelist:
		indexKey = keyIndexWhitelist
		keyPrefix = keyPrefixWhitelist
	default:
		return coreerrors.New(coreerrors.CodeStorageError, "invalid IP type: "+string(ipType))
	}

	// 删除记录
	key := keyPrefix + ip
	if err := m.storage.Delete(key); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete record")
	}

	// 从索引移除
	listStore, ok := m.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeStorageError, "storage does not support list operations")
	}
	if err := listStore.RemoveFromList(indexKey, ip); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to remove from index")
	}

	return nil
}

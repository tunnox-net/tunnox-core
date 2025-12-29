package security

import (
	"encoding/json"
	"fmt"
	"time"
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
		return fmt.Errorf("storage not available")
	}

	// 加载黑名单
	if err := m.loadListFromStorage(IPTypeBlacklist); err != nil {
		return fmt.Errorf("failed to load blacklist: %w", err)
	}

	// 加载白名单
	if err := m.loadListFromStorage(IPTypeWhitelist); err != nil {
		return fmt.Errorf("failed to load whitelist: %w", err)
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
		return fmt.Errorf("invalid IP type: %s", ipType)
	}

	// 获取IP列表
	listStore, ok := m.storage.(storage.ListStore)
	if !ok {
		return fmt.Errorf("storage does not support list operations")
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
		return fmt.Errorf("storage not available")
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
		return fmt.Errorf("invalid IP type: %s", ipType)
	}

	// 保存记录
	key := keyPrefix + ip
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to encode record: %w", err)
	}

	var ttl time.Duration
	if !record.ExpiresAt.IsZero() {
		ttl = time.Until(record.ExpiresAt)
		if ttl < 0 {
			ttl = 0
		}
	}

	if err := m.storage.Set(key, data, ttl); err != nil {
		return fmt.Errorf("failed to save record: %w", err)
	}

	// 添加到索引
	listStore, ok := m.storage.(storage.ListStore)
	if !ok {
		return fmt.Errorf("storage does not support list operations")
	}
	if err := listStore.AppendToList(indexKey, ip); err != nil {
		return fmt.Errorf("failed to add to index: %w", err)
	}

	return nil
}

// removeFromStorage 从Storage删除
func (m *IPManager) removeFromStorage(ipType IPType, ip string) error {
	if m.storage == nil {
		return fmt.Errorf("storage not available")
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
		return fmt.Errorf("invalid IP type: %s", ipType)
	}

	// 删除记录
	key := keyPrefix + ip
	if err := m.storage.Delete(key); err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}

	// 从索引移除
	listStore, ok := m.storage.(storage.ListStore)
	if !ok {
		return fmt.Errorf("storage does not support list operations")
	}
	if err := listStore.RemoveFromList(indexKey, ip); err != nil {
		return fmt.Errorf("failed to remove from index: %w", err)
	}

	return nil
}

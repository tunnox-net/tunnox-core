package fixtures

import (
	"encoding/json"
	"os"
	"path/filepath"

	"tunnox-core/internal/cloud/models"
)

// LoadUsers 加载用户测试数据
func LoadUsers() ([]*models.User, error) {
	data, err := loadJSONFile("users.json")
	if err != nil {
		return nil, err
	}

	var users []*models.User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, err
	}

	return users, nil
}

// LoadClients 加载客户端测试数据
func LoadClients() ([]*models.Client, error) {
	data, err := loadJSONFile("clients.json")
	if err != nil {
		return nil, err
	}

	var clients []*models.Client
	if err := json.Unmarshal(data, &clients); err != nil {
		return nil, err
	}

	return clients, nil
}

// LoadMappings 加载映射测试数据
func LoadMappings() ([]*models.PortMapping, error) {
	data, err := loadJSONFile("mappings.json")
	if err != nil {
		return nil, err
	}

	var mappings []*models.PortMapping
	if err := json.Unmarshal(data, &mappings); err != nil {
		return nil, err
	}

	return mappings, nil
}

// loadJSONFile 加载JSON文件
func loadJSONFile(filename string) ([]byte, error) {
	// 获取当前工作目录
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// 尝试在多个可能的路径中查找文件
	possiblePaths := []string{
		filepath.Join(currentDir, "tests", "fixtures", filename),
		filepath.Join(currentDir, "fixtures", filename),
		filename,
	}

	for _, path := range possiblePaths {
		if data, err := os.ReadFile(path); err == nil {
			return data, nil
		}
	}

	return nil, os.ErrNotExist
}


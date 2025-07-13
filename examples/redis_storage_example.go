package main

import (
	"context"
	"fmt"
	"log"
	"time"
	"tunnox-core/internal/cloud/storages"
)

func main() {
	ctx := context.Background()

	// 示例1: 使用Redis存储
	fmt.Println("=== Redis Storage Example ===")

	// 创建Redis配置
	redisConfig := &storages.RedisConfig{
		Addr:     "localhost:6379", // Redis服务器地址
		Password: "",               // Redis密码（如果有）
		DB:       0,                // 数据库编号
		PoolSize: 10,               // 连接池大小
	}

	// 创建Redis存储实例
	redisStorage, err := storages.NewRedisStorage(ctx, redisConfig)
	if err != nil {
		log.Printf("Failed to create Redis storage: %v", err)
		log.Println("Note: Make sure Redis server is running on localhost:6379")
		return
	}
	defer redisStorage.Close()

	// 基本操作示例
	fmt.Println("\n1. Basic Operations:")

	// 设置值
	key := "example:user:123"
	userData := map[string]interface{}{
		"name":     "John Doe",
		"email":    "john@example.com",
		"age":      30,
		"isActive": true,
	}

	err = redisStorage.Set(key, userData, 30*time.Minute)
	if err != nil {
		log.Printf("Failed to set key: %v", err)
		return
	}
	fmt.Printf("Set user data for key: %s\n", key)

	// 获取值
	retrieved, err := redisStorage.Get(key)
	if err != nil {
		log.Printf("Failed to get key: %v", err)
		return
	}
	fmt.Printf("Retrieved user data: %+v\n", retrieved)

	// 检查键是否存在
	exists, err := redisStorage.Exists(key)
	if err != nil {
		log.Printf("Failed to check existence: %v", err)
		return
	}
	fmt.Printf("Key exists: %v\n", exists)

	// 列表操作示例
	fmt.Println("\n2. List Operations:")

	listKey := "example:users:online"
	onlineUsers := []interface{}{"user1", "user2", "user3"}

	err = redisStorage.SetList(listKey, onlineUsers, 10*time.Minute)
	if err != nil {
		log.Printf("Failed to set list: %v", err)
		return
	}
	fmt.Printf("Set online users list: %v\n", onlineUsers)

	// 追加用户到列表
	err = redisStorage.AppendToList(listKey, "user4")
	if err != nil {
		log.Printf("Failed to append to list: %v", err)
		return
	}
	fmt.Println("Appended user4 to online users")

	// 获取完整列表
	allUsers, err := redisStorage.GetList(listKey)
	if err != nil {
		log.Printf("Failed to get list: %v", err)
		return
	}
	fmt.Printf("All online users: %v\n", allUsers)

	// 哈希操作示例
	fmt.Println("\n3. Hash Operations:")

	hashKey := "example:user:profile:123"

	// 设置哈希字段
	err = redisStorage.SetHash(hashKey, "name", "John Doe")
	if err != nil {
		log.Printf("Failed to set hash field: %v", err)
		return
	}

	err = redisStorage.SetHash(hashKey, "email", "john@example.com")
	if err != nil {
		log.Printf("Failed to set hash field: %v", err)
		return
	}

	err = redisStorage.SetHash(hashKey, "age", 30)
	if err != nil {
		log.Printf("Failed to set hash field: %v", err)
		return
	}

	fmt.Println("Set user profile hash fields")

	// 获取单个字段
	name, err := redisStorage.GetHash(hashKey, "name")
	if err != nil {
		log.Printf("Failed to get hash field: %v", err)
		return
	}
	fmt.Printf("User name: %v\n", name)

	// 获取所有字段
	profile, err := redisStorage.GetAllHash(hashKey)
	if err != nil {
		log.Printf("Failed to get all hash fields: %v", err)
		return
	}
	fmt.Printf("Complete profile: %+v\n", profile)

	// 计数器操作示例
	fmt.Println("\n4. Counter Operations:")

	counterKey := "example:page:views"

	// 递增计数器
	views, err := redisStorage.Incr(counterKey)
	if err != nil {
		log.Printf("Failed to increment counter: %v", err)
		return
	}
	fmt.Printf("Page views: %d\n", views)

	// 按值递增
	views, err = redisStorage.IncrBy(counterKey, 5)
	if err != nil {
		log.Printf("Failed to increment by value: %v", err)
		return
	}
	fmt.Printf("Page views after increment by 5: %d\n", views)

	// 原子操作示例
	fmt.Println("\n5. Atomic Operations:")

	atomicKey := "example:lock:resource"

	// 使用SetNX实现分布式锁
	acquired, err := redisStorage.SetNX(atomicKey, "locked", 30*time.Second)
	if err != nil {
		log.Printf("Failed to acquire lock: %v", err)
		return
	}
	fmt.Printf("Lock acquired: %v\n", acquired)

	// 尝试再次获取锁（应该失败）
	acquired2, err := redisStorage.SetNX(atomicKey, "locked", 30*time.Second)
	if err != nil {
		log.Printf("Failed to acquire lock again: %v", err)
		return
	}
	fmt.Printf("Second lock attempt: %v\n", acquired2)

	// 比较并交换操作
	casKey := "example:config:version"

	// 设置初始值
	err = redisStorage.Set(casKey, "v1.0", 1*time.Hour)
	if err != nil {
		log.Printf("Failed to set initial value: %v", err)
		return
	}

	// 比较并交换
	swapped, err := redisStorage.CompareAndSwap(casKey, "v1.0", "v2.0", 1*time.Hour)
	if err != nil {
		log.Printf("Failed to compare and swap: %v", err)
		return
	}
	fmt.Printf("Compare and swap successful: %v\n", swapped)

	// 验证交换结果
	version, err := redisStorage.Get(casKey)
	if err != nil {
		log.Printf("Failed to get version: %v", err)
		return
	}
	fmt.Printf("Current version: %v\n", version)

	// 过期时间操作示例
	fmt.Println("\n6. Expiration Operations:")

	expKey := "example:temp:data"

	// 设置带过期时间的值
	err = redisStorage.Set(expKey, "temporary data", 60*time.Second)
	if err != nil {
		log.Printf("Failed to set with expiration: %v", err)
		return
	}

	// 获取过期时间
	ttl, err := redisStorage.GetExpiration(expKey)
	if err != nil {
		log.Printf("Failed to get expiration: %v", err)
		return
	}
	fmt.Printf("Time to live: %v\n", ttl)

	// 更新过期时间
	err = redisStorage.SetExpiration(expKey, 120*time.Second)
	if err != nil {
		log.Printf("Failed to set new expiration: %v", err)
		return
	}
	fmt.Println("Updated expiration time to 120 seconds")

	// 使用存储工厂示例
	fmt.Println("\n7. Storage Factory Example:")

	factory := storages.NewStorageFactory(ctx)

	// 通过配置创建Redis存储
	config := map[string]interface{}{
		"type":      "redis",
		"addr":      "localhost:6379",
		"password":  "",
		"db":        0,
		"pool_size": 10,
	}

	factoryStorage, err := factory.CreateStorageWithConfig(config)
	if err != nil {
		log.Printf("Failed to create storage via factory: %v", err)
		return
	}
	defer factoryStorage.Close()

	// 测试工厂创建的存储
	factoryKey := "factory:test:key"
	factoryValue := "factory test value"

	err = factoryStorage.Set(factoryKey, factoryValue, 5*time.Minute)
	if err != nil {
		log.Printf("Failed to set via factory storage: %v", err)
		return
	}

	retrievedValue, err := factoryStorage.Get(factoryKey)
	if err != nil {
		log.Printf("Failed to get via factory storage: %v", err)
		return
	}
	fmt.Printf("Factory storage test: %v\n", retrievedValue)

	// 清理
	fmt.Println("\n8. Cleanup:")

	keysToDelete := []string{
		key, listKey, hashKey, counterKey, atomicKey, casKey, expKey, factoryKey,
	}

	for _, k := range keysToDelete {
		err := redisStorage.Delete(k)
		if err != nil {
			log.Printf("Failed to delete key %s: %v", k, err)
		} else {
			fmt.Printf("Deleted key: %s\n", k)
		}
	}

	fmt.Println("\n=== Example completed successfully ===")
}

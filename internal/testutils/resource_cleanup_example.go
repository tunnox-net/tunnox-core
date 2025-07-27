package testutils

import (
	"testing"
)

// ExampleWithCleanup 展示如何使用WithCleanup函数
func ExampleWithCleanup() {
	WithCleanup(func(cleanup *ResourceCleanup) {
		// 创建存储
		// storage := storage.NewMemoryStorage(context.Background())
		// cleanup.AddResource(storage)

		// 创建ID管理器
		// idManager := idgen.NewIDManager(storage, context.Background())
		// cleanup.AddResource(idManager)

		// 执行测试逻辑
		// ...

		// 函数结束时自动清理所有资源
	})
}

// ExampleResourceCleanup 展示如何手动使用ResourceCleanup
func ExampleResourceCleanup(t *testing.T) {
	cleanup := NewResourceCleanup()
	defer cleanup.DeferCleanup()()

	// 创建存储
	// storage := storage.NewMemoryStorage(context.Background())
	// cleanup.AddResource(storage)

	// 创建ID管理器
	// idManager := idgen.NewIDManager(storage, context.Background())
	// cleanup.AddResource(idManager)

	// 执行测试逻辑
	// ...

	// 测试结束时自动清理所有资源
}

// ExampleAddCloser 展示如何添加实现了Close方法的资源
func ExampleAddCloser() {
	WithCleanup(func(cleanup *ResourceCleanup) {
		// 添加实现了Close方法的资源
		// file, _ := os.Open("test.txt")
		// cleanup.AddCloser(file)

		// 添加实现了Dispose方法的资源
		// storage := storage.NewMemoryStorage(context.Background())
		// cleanup.AddResource(storage)

		// 执行测试逻辑
		// ...
	})
}

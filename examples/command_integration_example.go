package main

import (
	"context"
	"log"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/command"
	"tunnox-core/internal/protocol"
)

func ExampleCommandIntegration() {
	// 创建上下文
	ctx := context.Background()

	// 创建存储和ID管理器
	storage := storages.NewMemoryStorage(ctx)
	idManager := generators.NewIDManager(storage, ctx)

	// 创建会话
	session := protocol.NewConnectionSession(idManager, ctx)

	// 创建并配置命令注册表
	commandRegistry := command.CreateDefaultRegistry()

	// 设置命令注册表到会话
	session.SetCommandRegistry(commandRegistry)

	log.Println("Command system integrated successfully!")

	// 这里可以继续使用 session 进行其他操作
	// 例如：处理连接、处理命令等
}

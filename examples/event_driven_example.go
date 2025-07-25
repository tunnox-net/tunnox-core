package main

import (
	"context"
	"fmt"
	"log"
	"time"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/command"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
)

func main() {
	// 创建上下文
	ctx := context.Background()

	// 1. 创建事件总线
	eventBus := events.NewEventBus(ctx)
	defer eventBus.Close()

	// 2. 创建存储和ID管理器
	storage := storages.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(storage, ctx)
	defer idManager.Close()

	// 3. 创建Session管理器
	sessionManager := session.NewSessionManager(idManager, ctx)
	defer sessionManager.Close()

	// 4. 设置Session的事件总线
	if err := sessionManager.SetEventBus(eventBus); err != nil {
		log.Fatalf("Failed to set event bus for session: %v", err)
	}

	// 5. 创建命令服务
	commandService := command.NewCommandService(ctx)
	defer commandService.Close()

	// 6. 设置命令服务的事件总线
	if err := commandService.SetEventBus(eventBus); err != nil {
		log.Fatalf("Failed to set event bus for command service: %v", err)
	}

	// 7. 注册命令处理器
	registerCommandHandlers(commandService)

	// 8. 启动命令服务
	if err := commandService.Start(); err != nil {
		log.Fatalf("Failed to start command service: %v", err)
	}

	// 8. 注册自定义事件处理器
	registerCustomEventHandlers(eventBus)

	fmt.Println("🚀 事件驱动架构示例启动成功！")
	fmt.Println("📡 监听事件中...")

	// 9. 模拟一些操作
	simulateOperations(sessionManager, eventBus)

	// 10. 等待一段时间让事件处理完成
	time.Sleep(2 * time.Second)
	fmt.Println("✅ 示例完成！")
}

// registerCommandHandlers 注册命令处理器
func registerCommandHandlers(commandService command.CommandService) {
	// 注册TCP映射处理器
	tcpMapHandler := command.NewTcpMapHandler()
	if err := commandService.RegisterHandler(tcpMapHandler); err != nil {
		log.Printf("Failed to register TCP map handler: %v", err)
	}

	// 注册HTTP映射处理器
	httpMapHandler := command.NewHttpMapHandler()
	if err := commandService.RegisterHandler(httpMapHandler); err != nil {
		log.Printf("Failed to register HTTP map handler: %v", err)
	}

	// 注册SOCKS映射处理器
	socksMapHandler := command.NewSocksMapHandler()
	if err := commandService.RegisterHandler(socksMapHandler); err != nil {
		log.Printf("Failed to register SOCKS map handler: %v", err)
	}

	// 注册数据传输处理器
	dataInHandler := command.NewDataInHandler()
	if err := commandService.RegisterHandler(dataInHandler); err != nil {
		log.Printf("Failed to register data in handler: %v", err)
	}

	dataOutHandler := command.NewDataOutHandler()
	if err := commandService.RegisterHandler(dataOutHandler); err != nil {
		log.Printf("Failed to register data out handler: %v", err)
	}

	// 注册转发处理器
	forwardHandler := command.NewForwardHandler()
	if err := commandService.RegisterHandler(forwardHandler); err != nil {
		log.Printf("Failed to register forward handler: %v", err)
	}

	// 注册断开连接处理器
	disconnectHandler := command.NewDisconnectHandler()
	if err := commandService.RegisterHandler(disconnectHandler); err != nil {
		log.Printf("Failed to register disconnect handler: %v", err)
	}

	// 注册RPC调用处理器
	rpcHandler := command.NewRpcInvokeHandler()
	if err := commandService.RegisterHandler(rpcHandler); err != nil {
		log.Printf("Failed to register RPC invoke handler: %v", err)
	}

	// 注册默认处理器
	defaultHandler := command.NewDefaultHandler()
	if err := commandService.RegisterHandler(defaultHandler); err != nil {
		log.Printf("Failed to register default handler: %v", err)
	}

	fmt.Println("📝 已注册所有命令处理器")
}

// registerCustomEventHandlers 注册自定义事件处理器
func registerCustomEventHandlers(eventBus events.EventBus) {
	// 监听所有事件类型
	eventTypes := []string{
		"ConnectionEstablished",
		"ConnectionClosed",
		"CommandReceived",
		"CommandCompleted",
		"Heartbeat",
		"DisconnectRequest",
	}

	for _, eventType := range eventTypes {
		handler := func(event events.Event) error {
			fmt.Printf("📨 收到事件: %s, 来源: %s, 时间: %s\n",
				event.Type(), event.Source(), event.Timestamp().Format("15:04:05"))
			return nil
		}

		if err := eventBus.Subscribe(eventType, handler); err != nil {
			log.Printf("Failed to subscribe to %s events: %v", eventType, err)
		}
	}

	// 特殊处理命令完成事件
	commandCompletedHandler := func(event events.Event) error {
		if cmdEvent, ok := event.(*events.CommandCompletedEvent); ok {
			status := "❌ 失败"
			if cmdEvent.Success {
				status = "✅ 成功"
			}
			fmt.Printf("🎯 命令完成: %s, 连接: %s, 处理时间: %v\n",
				status, cmdEvent.ConnectionID, cmdEvent.ProcessingTime)
		}
		return nil
	}

	if err := eventBus.Subscribe("CommandCompleted", commandCompletedHandler); err != nil {
		log.Printf("Failed to subscribe to CommandCompleted events: %v", err)
	}
}

// simulateOperations 模拟一些操作
func simulateOperations(sessionManager *session.SessionManager, eventBus events.EventBus) {
	fmt.Println("\n🔄 开始模拟操作...")

	// 模拟连接建立
	fmt.Println("\n1️⃣ 模拟连接建立...")
	conn, err := sessionManager.CreateConnection(nil, nil)
	if err != nil {
		log.Printf("Failed to create connection: %v", err)
		return
	}

	// 更新连接状态（触发连接建立事件）
	if err := sessionManager.UpdateConnectionState(conn.ID, types.StateConnected); err != nil {
		log.Printf("Failed to update connection state: %v", err)
	}

	// 模拟心跳
	fmt.Println("\n2️⃣ 模拟心跳...")
	heartbeatPacket := &packet.TransferPacket{
		PacketType: packet.Heartbeat,
	}
	if err := sessionManager.ProcessPacket(conn.ID, heartbeatPacket); err != nil {
		log.Printf("Failed to process heartbeat: %v", err)
	}

	// 模拟TCP映射创建命令
	fmt.Println("\n3️⃣ 模拟TCP映射创建命令...")
	commandPacket := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.TcpMapCreate,
			CommandId:   "cmd_123",
			Token:       "req_456",
			SenderId:    "client_1",
			ReceiverId:  "server_1",
			CommandBody: `{"port": 8080, "target": "localhost:3000"}`,
		},
	}
	if err := sessionManager.ProcessPacket(conn.ID, commandPacket); err != nil {
		log.Printf("Failed to process command: %v", err)
	}

	// 等待命令处理完成
	time.Sleep(500 * time.Millisecond)

	// 模拟断开连接命令
	fmt.Println("\n4️⃣ 模拟断开连接命令...")
	disconnectPacket := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.Disconnect,
			CommandId:   "cmd_789",
			Token:       "req_101",
			SenderId:    "client_1",
			ReceiverId:  "server_1",
			CommandBody: `{"reason": "user_request"}`,
		},
	}
	if err := sessionManager.ProcessPacket(conn.ID, disconnectPacket); err != nil {
		log.Printf("Failed to process disconnect command: %v", err)
	}

	fmt.Println("\n🎉 所有操作模拟完成！")
}

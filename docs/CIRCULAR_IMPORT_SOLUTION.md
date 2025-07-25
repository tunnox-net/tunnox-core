# 循环导入问题解决方案

## 问题描述

在重构命令系统时，遇到了 `protocol` 包和 `command` 包之间的循环导入问题：

```
protocol → command → common → protocol
```

具体表现为：
- `protocol` 包需要导入 `command` 包来使用命令处理器
- `command` 包需要导入 `common` 包来使用 `Session` 接口
- `common` 包定义了 `Session` 接口
- `protocol` 包实现了 `Session` 接口

## 解决方案

### 1. 接口分离原则

将命令相关的接口定义移到 `common` 包中，避免循环依赖：

```go
// internal/common/command_interfaces.go
package common

// CommandHandler 命令处理器接口
type CommandHandler interface {
    Handle(ctx *CommandContext) (*CommandResponse, error)
    GetResponseType() CommandResponseType
    GetCommandType() packet.CommandType
    GetCategory() CommandCategory
    GetDirection() CommandDirection
}

// CommandRegistry 命令注册表接口
type CommandRegistry interface {
    Register(handler CommandHandler) error
    Unregister(commandType packet.CommandType) error
    GetHandler(commandType packet.CommandType) (CommandHandler, bool)
    ListHandlers() []packet.CommandType
    GetHandlerCount() int
}

// 其他相关接口和类型...
```

### 2. 依赖注入模式

使用依赖注入来打破循环依赖：

```go
// internal/protocol/session.go
type ConnectionSession struct {
    // ...
    commandRegistry CommandRegistry // 接口类型，不是具体实现
}

// 通过 SetCommandRegistry 方法注入依赖
func (s *ConnectionSession) SetCommandRegistry(registry CommandRegistry) {
    s.commandRegistry = registry
    s.registerDefaultHandlers()
}
```

### 3. 工厂模式

在 `command` 包中提供工厂函数来创建和配置注册表：

```go
// internal/command/factory.go
func CreateDefaultRegistry() common.CommandRegistry {
    registry := NewCommandRegistry()
    RegisterDefaultHandlers(registry)
    return registry
}
```

### 4. 类型别名

在 `command` 包中使用类型别名来引用 `common` 包中的接口：

```go
// internal/command/types.go
type CommandHandler = common.CommandHandler
type CommandContext = common.CommandContext
type CommandResponse = common.CommandResponse
type CommandRegistry = common.CommandRegistry
// ...
```

## 新的依赖关系

解决后的依赖关系：

```
common (定义接口)
  ↑
command (实现接口，使用类型别名)
  ↑
protocol (使用接口，通过依赖注入)
```

## 使用方式

### 1. 创建会话和注册表

```go
// 创建会话
session := protocol.NewConnectionSession(idManager, ctx)

// 创建并配置命令注册表
commandRegistry := command.CreateDefaultRegistry()

// 设置命令注册表到会话
session.SetCommandRegistry(commandRegistry)
```

### 2. 注册自定义处理器

```go
// 创建注册表
registry := command.NewCommandRegistry()

// 注册处理器
registry.Register(NewTcpMapHandler())
registry.Register(NewHttpMapHandler())

// 设置到会话
session.SetCommandRegistry(registry)
```

## 优势

1. **解耦**: `protocol` 包不再直接依赖 `command` 包的具体实现
2. **可测试**: 可以轻松注入模拟的处理器进行测试
3. **可扩展**: 可以动态注册和替换处理器
4. **类型安全**: 保持了完整的类型检查
5. **向后兼容**: 不影响现有的 API

## 注意事项

1. **初始化顺序**: 必须先创建注册表并注册处理器，然后设置到会话
2. **空值检查**: 在使用 `commandRegistry` 前要检查是否为 `nil`
3. **接口实现**: 所有处理器必须实现 `common.CommandHandler` 接口

## 测试验证

所有测试都通过，证明解决方案有效：

```bash
go test ./internal/command -v  # 通过
go test ./internal/protocol -v # 通过
go build ./...                 # 通过
``` 
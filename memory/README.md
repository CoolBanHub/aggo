# Memory 模块说明

`memory` 包已经从单一 `MemoryManager` 入口演进为 provider + middleware 的组合式设计：

- `MemoryProvider` 负责在模型调用前检索上下文、在模型调用后写入记忆
- `MemoryMiddleware` 负责把 provider 接到 `agent.AgentBuilder`
- `Registry` 负责注册和创建不同的记忆后端

如果你在写新代码，优先直接使用 `memory` 包的 provider API；`memory/compat.go` 里的 `MemoryManager`、`MemoryConfig` 等导出主要用于兼容旧调用方式。

## 核心接口

`memory/provider.go` 定义了统一接口：

```go
type MemoryProvider interface {
    Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResult, error)
    Memorize(ctx context.Context, req *MemorizeRequest) error
    Close() error
}
```

其中：

- `Retrieve` 在模型调用前执行，返回要注入的系统消息和历史消息
- `Memorize` 在模型返回后执行，保存本轮对话
- `Close` 用于释放底层资源

`memory/middleware.go` 会在 Agent 生命周期里自动调用这两个方法，因此业务侧通常不需要直接手动调用 `Retrieve` / `Memorize`。

## 接入方式

最简用法是创建 provider，然后通过 `WithMemory(provider)` 接入：

```go
package main

import (
    "context"

    "github.com/CoolBanHub/aggo/agent"
    "github.com/CoolBanHub/aggo/memory"
    "github.com/CoolBanHub/aggo/memory/builtin"
    "github.com/CoolBanHub/aggo/memory/builtin/storage"
    "github.com/CoolBanHub/aggo/model"
    "github.com/cloudwego/eino/adk"
    "github.com/cloudwego/eino/schema"
)

func main() {
    ctx := context.Background()

    chatModel, _ := model.NewChatModel(
        model.WithBaseUrl("https://api.openai.com/v1"),
        model.WithAPIKey("your-api-key"),
        model.WithModel("gpt-4"),
    )

    provider, _ := memory.GlobalRegistry().CreateProvider("builtin", &builtin.ProviderConfig{
        ChatModel: chatModel,
        Storage:   storage.NewMemoryStore(),
        MemoryConfig: &builtin.MemoryConfig{
            EnableUserMemories:   true,
            EnableSessionSummary: true,
            MemoryLimit:          8,
            Retrieval:            builtin.RetrievalLastN,
        },
    })
    defer provider.Close()

    ag, _ := agent.NewAgentBuilder(chatModel).
        WithInstruction("你是一个有记忆能力的助手").
        WithMemory(provider).
        Build(ctx)

    runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: ag})
    _ = runner.Run(ctx, []*schema.Message{
        schema.UserMessage("记住我叫 Alice，我喜欢摄影"),
    }, adk.WithSessionValues(map[string]any{
        "userID":    "alice",
        "sessionID": "demo-session",
    }))
}
```

关键点只有两个：

- 必须传入 `userID` 和 `sessionID`，否则 `MemoryMiddleware` 会直接跳过
- 创建 provider 后要 `defer provider.Close()`

## 已注册的 provider

当前仓库内默认注册了三个 provider：

- `builtin`
- `memu`
- `mem0`

可以通过 `memory.GlobalRegistry().ListPlugins()` 查看当前已注册插件。

### builtin

`builtin` 是仓库内置的完整记忆实现，注册逻辑在 `memory/builtin_adapter.go`。

它的特点：

- 基于 `builtin.MemoryManager` 适配到 `MemoryProvider`
- 支持用户长期记忆、会话摘要、历史消息三类数据
- 支持异步分析、摘要触发、周期清理
- 支持内存、文件、GORM 三种存储

创建方式：

```go
provider, err := memory.GlobalRegistry().CreateProvider("builtin", &builtin.ProviderConfig{
    ChatModel: chatModel,
    Storage:   storage.NewMemoryStore(),
    MemoryConfig: &builtin.MemoryConfig{
        EnableUserMemories:   true,
        EnableSessionSummary: true,
        MemoryLimit:          10,
        Retrieval:            builtin.RetrievalLastN,
    },
})
```

#### builtin 配置项

最常用的是 `builtin.MemoryConfig`：

- `EnableUserMemories`: 是否启用用户长期记忆
- `EnableSessionSummary`: 是否启用会话摘要
- `Retrieval`: 检索方式，支持 `RetrievalLastN`、`RetrievalFirstN`、`RetrievalSemantic`
- `MemoryLimit`: 历史消息检索上限
- `AsyncWorkerPoolSize`: 异步处理 worker 数量
- `SummaryTrigger`: 摘要触发策略
- `Cleanup`: 定期清理配置
- `TablePre`: SQL 表前缀

默认配置来自 `builtin.DefaultMemoryConfig()`。

#### builtin 存储实现

`memory/builtin/storage/` 目前提供三种实现：

- `storage.NewMemoryStore()`: 纯内存，适合测试或本地开发
- `storage.NewFileStore(dir, maxSessionMessages)`: 基于 JSONL 文件
- `storage.NewGormStorage(db)`: 基于 GORM，支持 MySQL / PostgreSQL / SQLite

如果使用 SQL 存储，`NewMemoryManager` 在初始化时会自动执行 `AutoMigrate()`。

### memu

`memu` 是一个外部 HTTP 记忆服务 provider，注册逻辑在 `memory/memu/provider.go`。

它的工作方式：

- 检索阶段向 `POST /retrieve` 发请求
- 写入阶段向 `POST /memorize` 发请求
- 检索失败时会优雅降级，返回空结果，不阻塞主流程

创建方式：

```go
package main

import (
    "github.com/CoolBanHub/aggo/memory"
    "github.com/CoolBanHub/aggo/memory/memu"
)

provider, err := memory.GlobalRegistry().CreateProvider("memu", &memu.ProviderConfig{
    BaseURL:      "http://127.0.0.1:8000",
    HistoryLimit: 6,
    MaxItems:     3,
})
```

常用配置：

- `BaseURL`: memu 服务地址，必填
- `HistoryLimit`: 检索请求最多带多少条最近消息
- `MaxItems`: 最多向模型注入多少条 memu 返回的记忆条目

### mem0

`mem0` 是一个兼容 mem0 Hosted API 和 OSS REST API 的外部记忆 provider，注册逻辑在 `memory/mem0/provider.go`。

它的工作方式：

- 检索阶段向 mem0 的 search API 发请求
- 写入阶段向 mem0 的 add memories API 发请求
- 兼容 hosted / oss 两种模式，并允许覆盖默认路径和鉴权 header
- 检索失败时会优雅降级，返回空结果，不阻塞主流程
- 对部分 hosted 服务，写入可能是异步完成的；新写入的记忆不一定能在下一秒立即被检索到

创建方式：

```go
package main

import (
    "github.com/CoolBanHub/aggo/memory"
    "github.com/CoolBanHub/aggo/memory/mem0"
)

provider, err := memory.GlobalRegistry().CreateProvider("mem0", &mem0.ProviderConfig{
    BaseURL:           "https://api.mem0.ai",
    APIKey:            "your-mem0-api-key",
    Mode:              mem0.ModeHosted,
    SearchMsgLimit:    6,
    SearchResultLimit: 5,
    OutputMemoryLimit: 5,
})
```

如果是自建或第三方兼容接口，可以切到 OSS 模式：

```go
provider, err := memory.GlobalRegistry().CreateProvider("mem0", &mem0.ProviderConfig{
    BaseURL: "http://127.0.0.1:8000",
    Mode:    mem0.ModeOSS,
    APIKey:  "optional-api-key",
})
```

常用配置：

- `BaseURL`: mem0 服务地址，必填
- `Mode`: `mem0.ModeHosted` 或 `mem0.ModeOSS`
- `APIKey`: API key；hosted 默认使用 `Authorization: Token <key>`，oss 默认使用 `X-API-Key`
- `AddPath` / `SearchPath`: 覆盖默认接口路径，适配第三方兼容服务
- `AuthHeader` / `AuthScheme`: 覆盖默认鉴权 header 或 scheme
- `SearchMsgLimit`: 构造检索 query 时最多读取多少条最近消息
- `SearchResultLimit`: 搜索 API 期望返回多少条结果
- `OutputMemoryLimit`: 最多向模型注入多少条 mem0 记忆
- `UseSessionAsRunID`: 是否把 `sessionID` 透传为 mem0 的 `run_id`
- `SearchBySession`: 是否在检索阶段也按 `run_id` 过滤
- `SessionMetadataKey`: 是否把 `sessionID` 同步写入 metadata，默认 key 为 `session_id`
- `AgentID` / `AppID` / `OrgID` / `ProjectID`: 透传 mem0 的作用域字段
- `ExtraHeaders`: 额外请求头
- `Metadata`: 每次写入时附带的固定 metadata

## 生命周期说明

`MemoryMiddleware` 的行为比较直接：

1. `BeforeModelRewriteState` 阶段调用 `provider.Retrieve`
2. 把 `SystemMessages` 和 `HistoryMessages` 拼到当前 `state.Messages`
3. `AfterModelRewriteState` 阶段提取最近一轮 `user + assistant` 消息
4. 异步调用 `provider.Memorize`

这意味着：

- provider 侧要假设 `Memorize` 可能在后台 goroutine 中执行
- 如果没有有效的 `user` 或 `assistant` 消息，本轮不会入库
- 同一个 middleware 实例会做一次“已注入”标记，避免重复注入记忆

## 迁移建议

旧 README 或旧业务代码里可能还会看到这类写法：

```go
memoryManager, _ := memory.NewMemoryManager(...)
```

这条路径仍然可用，但新代码更推荐：

```go
provider, _ := memory.GlobalRegistry().CreateProvider("builtin", ...)
ag, _ := agent.NewAgentBuilder(chatModel).WithMemory(provider).Build(ctx)
```

这样做的好处是：

- 业务层只依赖统一的 `MemoryProvider`
- 更容易切换到 `memu` 或自定义 provider
- `agent` 层不需要知道具体的记忆实现细节

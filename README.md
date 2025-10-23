# AGGO - AI Agent Go Framework

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.24-blue)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![CloudWeGo Eino](https://img.shields.io/badge/powered%20by-CloudWeGo%20Eino-orange)](https://github.com/cloudwego/eino)

AGGO 是一个基于 Go 语言和 [CloudWeGo Eino](https://github.com/cloudwego/eino) 框架构建的企业级 AI Agent 框架，提供完整的对话 AI、知识管理、记忆系统和工具调用能力。

## ✨ 核心特性

### 🤖 智能代理系统
- **React 模式代理**: 基于 CloudWeGo Eino ADK 的 ReAct (Reasoning + Acting) 模式实现
- **工具调用**: 原生支持多种工具集成，包括知识库、数据库、Shell 命令等
- **多轮对话**: 上下文感知的多轮对话能力
- **流式响应**: 基于 SSE (Server-Sent Events) 的实时流式输出

### 🧠 记忆管理系统
- **会话记忆**: 自动管理会话级别的对话历史
- **长期记忆**: 支持用户级别的长期记忆存储
- **智能摘要**: 自动生成会话摘要，优化上下文长度
- **多种检索策略**: 支持 LastN、全部、摘要等多种记忆检索模式
- **灵活存储**: 支持内存存储和 SQL 存储（MySQL、PostgreSQL、SQLite）

### 📚 向量数据库集成
- **Milvus**: 企业级向量数据库支持，适合大规模生产环境
- **PostgreSQL + pgvector**: 轻量级向量搜索方案
- **统一接口**: 提供一致的向量存储和检索 API

### 🛠️ 丰富的工具生态
- **知识库工具**: 文档加载、语义搜索、向量检索
- **数据库工具**: MySQL、PostgreSQL 操作工具
- **Shell 工具**: 安全的系统命令执行
- **可扩展**: 易于集成自定义工具

### 📊 可观测性
- **Langfuse 集成**: AI 应用监控和追踪
- **日志管理**: 结构化日志记录
- **性能监控**: 支持 OpenTelemetry 追踪

## 🏗️ 系统架构

```
┌──────────────────────────────────────────────────────────────────┐
│                        AGGO Framework                             │
│                                                                   │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   Agent Layer   │  │  Memory Layer   │  │   Tool Layer    │  │
│  │                 │  │                 │  │                 │  │
│  │ • ReAct Agent   │◄─┤ • Session Mem   │  │ • Knowledge     │  │
│  │ • Multi-turn    │  │ • Long-term Mem │  │ • Database      │  │
│  │ • Streaming     │  │ • Auto Summary  │  │ • Shell Exec    │  │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘  │
│           │                    │                    │            │
│           └────────────────────┼────────────────────┘            │
│                                │                                 │
│           ┌────────────────────┴────────────────────┐            │
│           │         Storage & Vector Layer          │            │
│           │                                          │            │
│           │  ┌──────────────┐  ┌─────────────────┐  │            │
│           │  │   Vector DB  │  │  Memory Store   │  │            │
│           │  │              │  │                 │  │            │
│           │  │ • Milvus     │  │ • In-Memory     │  │            │
│           │  │ • PostgreSQL │  │ • SQL (GORM)    │  │            │
│           │  └──────────────┘  └─────────────────┘  │            │
│           └──────────────────────────────────────────┘            │
│                                                                   │
│           ┌──────────────────────────────────────────┐            │
│           │         Model & Embedding Layer          │            │
│           │                                          │            │
│           │  • OpenAI Compatible Chat Models         │            │
│           │  • OpenAI Compatible Embedding Models    │            │
│           │  • Support Reasoning Parameters          │            │
│           └──────────────────────────────────────────┘            │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │   Observability Layer   │
                    │                         │
                    │  • Langfuse Tracing     │
                    │  • Structured Logging   │
                    │  • SSE Event Streaming  │
                    └─────────────────────────┘
```

## 📦 安装

### 前置要求

- **Go**: >= 1.24.0
- **向量数据库** (二选一):
  - [Milvus](https://milvus.io/) >= 2.6 (推荐用于生产环境)
  - [PostgreSQL](https://www.postgresql.org/) >= 14 + [pgvector](https://github.com/pgvector/pgvector) 扩展
- **AI 模型服务**:
  - OpenAI API 兼容的服务 (OpenAI, Azure OpenAI, 或其他兼容服务)
- **可选依赖**:
  - [Langfuse](https://langfuse.com/) - AI 应用监控和追踪

### 安装框架

```bash
go get github.com/CoolBanHub/aggo
```

### 安装依赖

```bash
go mod download
```

## 🚀 快速开始

### 1. 基础 AI 代理示例

创建一个简单的对话代理：

```go
package main

import (
    "context"
    "log"

    "github.com/CoolBanHub/aggo/agent"
    "github.com/CoolBanHub/aggo/model"
    "github.com/CoolBanHub/aggo/memory"
    memoryStorage "github.com/CoolBanHub/aggo/memory/storage"
    "github.com/cloudwego/eino/schema"
)

func main() {
    ctx := context.Background()

    // 创建聊天模型
    cm, _ := model.NewChatModel(
        model.WithBaseUrl("https://api.openai.com/v1"),
        model.WithAPIKey("your-api-key"),
        model.WithModel("gpt-4"),
    )

    // 创建记忆管理器
    memoryStore := memoryStorage.NewMemoryStore()
    memoryManager, _ := memory.NewMemoryManager(cm, memoryStore, &memory.MemoryConfig{
        MemoryLimit: 10,
        Retrieval:   memory.RetrievalLastN,
    })

    // 创建代理
    agent, _ := agent.NewAgent(ctx, cm,
        agent.WithMemoryManager(memoryManager),
        agent.WithSystemPrompt("你是一个友好的 AI 助手"),
    )

    // 进行对话
    response, _ := agent.Generate(ctx, []*schema.Message{
        schema.UserMessage("你好，介绍一下你自己"),
    })

    log.Printf("AI: %s", response.Content)
}
```

### 2. 运行示例程序

```bash
# 知识库代理示例
go run example/knowledge_agent_tool_test/main.go

# 记忆系统示例
go run example/mem_agent_test/main.go

# SSE 流式响应示例
go run example/sse/main.go

# ADK 使用示例
go run example/adk_test/main.go
```

## 💡 核心功能详解

### 代理配置选项

AGGO 提供了灵活的代理配置选项：

```go
agent, err := agent.NewAgent(ctx, chatModel,
    agent.WithMemoryManager(memoryManager),       // 设置记忆管理器
    agent.WithSystemPrompt("你是一个AI助手"),      // 设置系统提示词
    agent.WithTools(tools...),                    // 添加工具
    agent.WithMaxStep(10),                         // 设置最大推理步数
)
```

### 记忆管理配置

```go
memoryConfig := &memory.MemoryConfig{
    EnableSessionSummary: true,              // 启用会话摘要
    EnableUserMemories:   true,              // 启用用户长期记忆
    MemoryLimit:          10,                // 记忆条数限制
    Retrieval:            memory.RetrievalLastN,  // 检索策略
}

memoryManager, err := memory.NewMemoryManager(
    chatModel,
    memoryStore,
    memoryConfig,
)
```

**记忆检索策略**:
- `RetrievalLastN`: 返回最近 N 条记忆
- `RetrievalAll`: 返回所有记忆
- `RetrievalSummary`: 仅返回摘要

### 向量数据库集成

#### Milvus 配置

```go
import (
    "github.com/CoolBanHub/aggo/database/milvus"
    "github.com/milvus-io/milvus/client/v2/milvusclient"
)

// 创建 Milvus 客户端
client, _ := milvusclient.New(ctx, &milvusclient.ClientConfig{
    Address: "127.0.0.1:19530",
    DBName:  "",  // 使用默认数据库
})

// 创建向量数据库实例
vectorDB, _ := milvus.NewMilvus(milvus.MilvusConfig{
    Client:         client,
    CollectionName: "knowledge_vectors",
    EmbeddingDim:   1024,
    Embedding:      embeddingModel,
})
```

#### PostgreSQL + pgvector 配置

```go
import "github.com/CoolBanHub/aggo/database/postgres"

vectorDB, _ := postgres.NewPostgres(postgres.PostgresConfig{
    Client:          gormDB,  // GORM 数据库实例
    CollectionName:  "knowledge_vectors",
    VectorDimension: 1024,
    Embedding:       embeddingModel,
})
```

### 模型配置

#### 聊天模型

```go
import "github.com/CoolBanHub/aggo/model"

chatModel, _ := model.NewChatModel(
    model.WithBaseUrl("https://api.openai.com/v1"),
    model.WithAPIKey("your-api-key"),
    model.WithModel("gpt-4"),
    model.WithReasoningEffort("medium"),  // 推理强度: low, medium, high
)
```

#### 嵌入模型

```go
embeddingModel, _ := model.NewEmbModel(
    model.WithBaseUrl("https://api.openai.com/v1"),
    model.WithAPIKey("your-api-key"),
    model.WithModel("text-embedding-3-large"),
    model.WithDimensions(1024),
)
```

### 工具集成

#### 知识库工具

```go
import "github.com/CoolBanHub/aggo/tools"

knowledgeTools := tools.GetKnowledgeTools(vectorDB, retriever, &retriever.Options{
    TopK:           utils.ValueToPtr(10),
    ScoreThreshold: utils.ValueToPtr(0.1),
})
```

**功能**:
- 文档加载 (支持文件和 URL)
- 语义搜索
- 向量检索

#### 数据库工具

```go
// MySQL 工具
mysqlTool := tools.GetMySQLTool(mysqlDB)

// PostgreSQL 工具
postgresTool := tools.GetPostgresTool(postgresDB)
```

#### Shell 工具

```go
shellTool := tools.GetShellTool()  // 安全的系统命令执行
```

### SSE 流式响应

```go
import "github.com/CoolBanHub/aggo/pkg/sse"

// 创建 SSE 写入器
writer := sse.NewSSEWriter(w, r)
defer writer.WriteDone()

// 流式生成
agent.Stream(ctx, messages,
    agent.WithStreamCallback(func(chunk string) {
        writer.WriteData(chunk)
    }),
)
```

## 🔧 环境变量配置

创建 `.env` 文件配置必要的环境变量：

```bash
# OpenAI API 配置
OPENAI_API_KEY=your-api-key
OPENAI_BASE_URL=https://api.openai.com/v1

# Milvus 配置
MILVUS_ADDRESS=127.0.0.1:19530

# Langfuse 配置 (可选)
LANGFUSE_SECRET_KEY=sk-lf-...
LANGFUSE_PUBLIC_KEY=pk-lf-...
LANGFUSE_HOST=https://cloud.langfuse.com
```

## 🛠️ 开发指南

### 项目结构

```
aggo/
├── agent/                      # AI 代理系统
│   ├── agent.go                   # ReAct 代理实现
│   ├── option.go                  # 代理配置选项
│   └── utils.go                   # 工具函数
│
├── memory/                     # 记忆管理系统
│   ├── manager.go                 # 记忆管理器
│   ├── storage.go                 # 存储接口
│   ├── types.go                   # 类型定义
│   ├── session_summary_generator.go  # 会话摘要生成器
│   ├── summary_trigger_manager.go    # 摘要触发管理器
│   ├── user_memory_analyzer.go       # 用户记忆分析器
│   └── storage/                   # 存储实现
│       ├── memory.go                 # 内存存储
│       ├── sql.go                    # SQL 存储 (GORM)
│       └── sql_models.go             # 数据模型
│
├── database/                   # 向量数据库
│   ├── database.go                # 数据库接口
│   ├── milvus/                    # Milvus 实现
│   │   ├── milvus.go                 # Milvus 客户端
│   │   ├── option.go                 # 配置选项
│   │   └── utils.go                  # 工具函数
│   └── postgres/                  # PostgreSQL + pgvector 实现
│       ├── postgres.go               # PostgreSQL 客户端
│       ├── option.go                 # 配置选项
│       └── utils.go                  # 工具函数
│
├── model/                      # AI 模型封装
│   ├── chat.go                    # 聊天模型 (支持推理强度参数)
│   ├── embedding.go               # 嵌入模型
│   └── option.go                  # 模型配置选项
│
├── tools/                      # 工具集
│   ├── knowledge_tool.go             # 知识库操作工具
│   ├── knowledge_reasoning_tools.go  # 知识推理工具
│   ├── mysql_tool.go                 # MySQL 数据库工具
│   ├── postgres_tool.go              # PostgreSQL 数据库工具
│   ├── shell_tool.go                 # Shell 命令执行工具
│   └── example_tools.go              # 示例工具
│
├── pkg/                        # 公共包
│   ├── sse/                       # Server-Sent Events
│   │   ├── sse.go                    # SSE 核心实现
│   │   ├── event.go                  # 事件定义
│   │   └── writer.go                 # SSE 写入器
│   └── langfuse/                  # Langfuse 可观测性
│       └── langfuse.go               # Langfuse 客户端
│
├── utils/                      # 工具函数
│   ├── utils.go                   # 通用工具
│   ├── uuid.go                    # UUID 生成
│   ├── ulid.go                    # ULID 生成
│   ├── float.go                   # 浮点数处理
│   └── convert.go                 # 类型转换
│
├── state/                      # 状态管理
│   └── chat.go                    # 聊天状态
│
├── config/                     # 配置管理
│   └── config.go                  # 配置定义
│
└── example/                    # 示例代码
    ├── knowledge_agent_tool_test/ # 知识库代理示例
    ├── mem_agent_test/            # 记忆系统示例
    ├── sse/                       # SSE 流式响应示例
    ├── adk_test/                  # ADK 使用示例
    ├── callback_test/             # 回调示例
    └── tool_test/                 # 工具测试示例
```

### 构建和测试

```bash
# 构建项目
go build ./...

# 运行测试
go test ./...

# 运行特定包测试
go test -v ./agent/...
go test -v ./memory/...
go test -v ./database/...

# 运行示例
go run example/knowledge_agent_tool_test/main.go
go run example/mem_agent_test/main.go
go run example/sse/main.go
```

## 🐛 故障排除

### 向量维度不匹配

**问题**: 向量维度不匹配导致插入失败

**解决方案**:
- 确保嵌入模型配置的 `Dimensions` 与向量数据库的 `EmbeddingDim` 一致
- 推荐统一使用 1024 维度 (`text-embedding-3-large` 模型)

### Milvus 连接失败

**问题**: 无法连接到 Milvus 服务

**解决方案**:
- 检查 Milvus 服务是否正常运行: `docker ps`
- 使用 `DBName: ""` 连接默认数据库
- 确认端口 19530 未被占用

### PostgreSQL pgvector 扩展未安装

**问题**: `extension "vector" does not exist`

**解决方案**:
```sql
-- 安装 pgvector 扩展
CREATE EXTENSION IF NOT EXISTS vector;

-- 验证安装
\dx vector
```

### 记忆管理器未正常关闭

**问题**: 程序退出时资源未释放

**解决方案**:
```go
defer memoryManager.Close()  // 确保在创建后立即添加 defer
```

## 🤝 贡献

我们欢迎各种形式的贡献！

### 如何贡献

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

### 贡献指南

- 代码需遵循 Go 语言规范
- 添加必要的单元测试
- 更新相关文档
- 保持提交信息清晰明了

## 📄 许可证

本项目采用 [MIT License](LICENSE) 开源许可证。

## 🙏 致谢

- [CloudWeGo Eino](https://github.com/cloudwego/eino) - 强大的 AI Agent 开发框架
- [Milvus](https://milvus.io/) - 高性能向量数据库
- [Langfuse](https://langfuse.com/) - AI 应用可观测性平台

## 📧 联系方式

- 问题反馈: [GitHub Issues](https://github.com/CoolBanHub/aggo/issues)
- 讨论交流: [GitHub Discussions](https://github.com/CoolBanHub/aggo/discussions)

---

<div align="center">

**AGGO** - 构建智能 AI Agent 的 Go 语言框架

[开始使用](#-快速开始) · [查看示例](./example) · [贡献代码](#-贡献)

Made with ❤️ by the AGGO Team

</div>
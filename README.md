# AGGO - AI智能代理框架

AGGO是一个基于Go语言构建的智能AI代理框架，集成了对话AI、知识管理、记忆系统和工具调用等功能，基于CloudWeGo Eino框架开发。

## 🚀 核心特性

- **智能对话代理**: 基于React模式的AI代理，支持工具调用和多轮对话
- **知识库管理**: 双重存储架构，结合传统数据库和向量数据库实现高效的语义搜索
- **记忆系统**: 会话级记忆管理，支持长期记忆存储和智能摘要
- **工具集成**: 丰富的工具生态，包括知识库操作、系统命令执行、数据库操作等
- **多数据库支持**: 支持SQLite、MySQL、PostgreSQL等多种数据库
- **向量搜索**: 支持Milvus和PostgreSQL向量数据库的语义相似度搜索
- **实时通信**: 支持Server-Sent Events (SSE) 流式响应
- **可观测性**: 集成Langfuse进行AI应用监控和追踪

## 🏗️ 系统架构

### 核心组件

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Agent系统     │    │   知识管理系统   │    │   记忆系统      │
│                 │    │                 │    │                 │
│ • 对话代理      │    │ • 文档处理      │    │ • 会话记忆      │
│ • 工具调用      │◄───┤ • 向量搜索      │    │ • 长期记忆      │
│ • React模式     │    │ • 语义检索      │    │ • 智能摘要      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────┐
                    │   存储层        │
                    │                 │
                    │ • GORM (元数据) │
                    │ • Milvus (向量) │
                    │ • Azure OpenAI │
                    └─────────────────┘
```

### 双重存储架构

- **存储层 (Storage)**: 使用GORM管理文档元数据、内容和时间戳
- **向量层 (VectorDB)**: 使用Milvus存储和搜索1024维度的向量数据
- **知识管理器**: 协调两个存储层，提供统一的知识库管理接口

## 📦 安装依赖

```bash
go mod download
```

### 外部依赖

- **向量数据库**: Milvus或PostgreSQL with pgvector扩展
- **关系型数据库**: MySQL、PostgreSQL或SQLite（可选其一）
- **AI服务**: Azure OpenAI用于聊天和嵌入向量生成
- **监控服务**: Langfuse（可选，用于AI应用监控）

## 🚀 快速开始

### 1. 基础知识库代理示例

```bash
go run example/knowledge_agent_tool_test/main.go
```

### 2. 记忆系统测试

```bash
go run example/mem_agent_test/main.go
```

### 3. SSE流式响应示例

```bash
go run example/sse/main.go
```

## 💡 使用示例

### 创建带知识库工具的AI代理

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/database/milvus"
	"github.com/CoolBanHub/aggo/memory"
	memoryStorage "github.com/CoolBanHub/aggo/memory/storage"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/tools"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/flow/retriever/router"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

func main() {
	ctx := context.Background()

	// 1. 创建聊天模型和嵌入模型
	cm, err := model.NewChatModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("gpt-4o-mini"),
	)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}

	em, err := model.NewEmbModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("text-embedding-3-large"),
		model.WithDimensions(1024),
	)
	if err != nil {
		log.Fatalf("创建嵌入模型失败: %v", err)
	}

	// 2. 创建 Milvus 向量数据库
	client, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address: "127.0.0.1:19530",
		DBName:  "", // 使用默认数据库
	})
	if err != nil {
		log.Fatalf("创建 Milvus 客户端失败: %v", err)
	}

	databaseDB, err := milvus.NewMilvus(milvus.MilvusConfig{
		Client:         client,
		CollectionName: "aggo_knowledge_vectors",
		EmbeddingDim:   1024,
		Embedding:      em,
	})
	if err != nil {
		log.Fatalf("创建数据库失败: %v", err)
	}

	// 3. 创建记忆管理器
	memoryStore := memoryStorage.NewMemoryStore()
	memoryManager, err := memory.NewMemoryManager(cm, memoryStore, &memory.MemoryConfig{
		EnableSessionSummary: false,
		EnableUserMemories:   false,
		MemoryLimit:          8,
		Retrieval:            memory.RetrievalLastN,
	})
	if err != nil {
		log.Fatalf("创建记忆管理器失败: %v", err)
	}
	defer memoryManager.Close()

	// 4. 创建检索路由器
	routerRetriever, err := router.NewRetriever(ctx, &router.Config{
		Retrievers: map[string]retriever.Retriever{
			"vector": databaseDB,
		},
		Router: func(ctx context.Context, query string) ([]string, error) {
			return []string{"vector"}, nil
		},
		FusionFunc: func(ctx context.Context, result map[string][]*schema.Document) ([]*schema.Document, error) {
			docsList := make([]*schema.Document, 0)
			for _, v := range result {
				docsList = append(docsList, v...)
			}
			return docsList, nil
		},
	})
	if err != nil {
		log.Fatalf("创建检索路由器失败: %v", err)
	}

	// 5. 创建带知识库工具的 AI 代理
	mainAgent, err := agent.NewAgent(ctx, cm,
		agent.WithMemoryManager(memoryManager),
		agent.WithTools(tools.GetKnowledgeTools(databaseDB, routerRetriever, &retriever.Options{
			TopK:           utils.ValueToPtr(10),
			ScoreThreshold: utils.ValueToPtr(0.1), // 默认相似度阈值
		})),
		agent.WithSystemPrompt("你是一个技术专家助手。当用户询问技术问题时，你应该使用 load_documents 和 search_documents 工具来加载和搜索相关信息。"),
	)
	if err != nil {
		log.Fatalf("创建 AI 代理失败: %v", err)
	}

	// 6. 使用 AI 代理进行对话
	userID := utils.GetUUIDNoDash()
	sessionID := utils.GetUUIDNoDash()

	// 用户可以通过对话要求 AI 加载文档和搜索
	response, err := mainAgent.Generate(ctx, []*schema.Message{
		schema.UserMessage("请加载一些关于Go语言的文档，然后告诉我Go语言的特点。"),
	}, agent.WithChatUserID(userID), agent.WithChatSessionID(sessionID))

	if err != nil {
		log.Fatalf("生成回答失败: %v", err)
	}

	log.Printf("AI助手: %s", response.Content)
}
```

### 知识库工具详解

AGGO 提供了两个核心知识库工具：

#### 1. load_documents 工具

用于将文档加载到知识库中，支持多种数据源：

**支持的文档来源**：

- `file`: 本地文件
- `url`: 网络URL

**使用示例**：

```go
// AI 可以通过自然语言调用此工具
response, err := agent.Generate(ctx, []*schema.Message{
schema.UserMessage("请加载 https://example.com/doc.pdf 这个文档到知识库"),
})
```

#### 2. search_documents 工具

用于在知识库中搜索相关文档：

**搜索配置**：

- **TopK**: 返回最相关的前K个结果（默认10个）
- **ScoreThreshold**: 相似度阈值（默认0.1）
- **支持向量相似度搜索**

**使用示例**：

```go
// AI 可以通过自然语言调用此工具
response, err := agent.Generate(ctx, []*schema.Message{
schema.UserMessage("搜索关于Go语言特性的文档"),
})
```

#### 工具配置选项

```go
// 创建知识库工具时可以自定义配置
tools.GetKnowledgeTools(databaseDB, routerRetriever, &retriever.Options{
TopK:           utils.ValueToPtr(10), // 搜索结果数量
ScoreThreshold: utils.ValueToPtr(0.1), // 相似度阈值
})
```

### 基本代理创建示例

```go
import (
"context"
"github.com/CoolBanHub/aggo/agent"
"github.com/CoolBanHub/aggo/model"
"github.com/CoolBanHub/aggo/memory"
memoryStorage "github.com/CoolBanHub/aggo/memory/storage"
)

func createBasicAgent() (*agent.Agent, error) {
ctx := context.Background()

// 创建聊天模型
cm, err := model.NewChatModel()
if err != nil {
return nil, err
}

// 创建记忆存储
memoryStore := memoryStorage.NewMemoryStore()
memoryManager, err := memory.NewMemoryManager(cm, memoryStore, &memory.MemoryConfig{
MemoryLimit: 10,
Retrieval:   memory.RetrievalLastN,
})
if err != nil {
return nil, err
}

// 创建基本代理
return agent.NewAgent(ctx, cm,
agent.WithMemoryManager(memoryManager),
agent.WithSystemPrompt("你是一个乐于助人的AI助手。"),
)
}
```

## 🔧 配置说明

### 向量维度配置

系统统一使用**1024维度**向量：

```go
// 嵌入模型配置
em, err := model.NewEmbModel(
model.WithModel("text-embedding-3-large"),
model.WithDimensions(1024), // 限制输出维度为1024
)

// Milvus配置
milvusConfig := milvus.MilvusConfig{
EmbeddingDim: 1024, // 匹配嵌入维度
Embedding:    em,
}
```

### 数据库配置

#### SQLite (开发环境)

```go
storage, err := storage.NewSQLiteStorage("knowledge.db")
```

#### MySQL (生产环境)

```go
storage, err := storage.NewMySQLStorage("localhost", 3306, "aggo", "user", "password")
```

#### 向量数据库配置

**Milvus向量数据库:**

```go
// 创建 Milvus 客户端
client, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
Address: "127.0.0.1:19530",
DBName:  "", // 空字符串使用默认数据库
})

// 创建 Milvus 数据库实例
milvusDB, err := milvus.NewMilvus(milvus.MilvusConfig{
Client:         client,
CollectionName: "aggo_knowledge_vectors",
EmbeddingDim:   1024,
Embedding:      em, // 嵌入模型实例
})
```

**PostgreSQL向量数据库:**

```go
// 创建 PostgreSQL 数据库实例
postgresDB, err := postgres.NewPostgres(postgres.PostgresConfig{
Client:          gormDB, // GORM 数据库实例
CollectionName:  "aggo_knowledge_vectors",
VectorDimension: 1024,
Embedding:       em, // 嵌入模型实例
})
```

## 🛠️ 开发指南

### 项目结构

```
aggo/
├── agent/              # AI代理系统
│   ├── agent.go           # 主代理实现 (已重构消息处理与内存管理)
│   ├── knowledge_agent.go # 知识型代理
│   └── option.go          # 配置选项
├── knowledge/          # 知识管理系统
│   ├── manager.go         # 知识库管理器
│   ├── interfaces.go      # 接口定义
│   ├── storage/           # 存储层
│   ├── vectordb/          # 向量数据库
│   └── readers/           # 文档读取器
├── memory/             # 记忆系统
├── model/              # AI模型封装
│   ├── chat.go            # 聊天模型
│   └── embedding.go       # 嵌入模型
├── tools/              # 工具集
│   ├── knowledge_tool.go      # 知识管理工具 (已更新相似度阈值)
│   ├── knowledge_reasoning_tools.go # 知识推理工具
│   ├── mysql_tool.go          # MySQL数据库工具
│   ├── postgres_tool.go       # PostgreSQL数据库工具
│   └── shell_tool.go          # 系统命令工具
├── pkg/                # 公共包
│   ├── sse/               # Server-Sent Events支持
│   └── langfuse/          # Langfuse监控集成
└── example/            # 示例代码
```

### 构建和测试

```bash
# 构建所有包
go build ./...

# 运行所有测试
go test ./...

# 运行特定模块测试
go test -v ./knowledge/...
go test -v ./agent/...

# 运行示例
go run example/knowledge_agent_tool_test/main.go

# 运行SSE示例
go run example/sse/main.go
```

## 🐛 常见问题

### 向量维度不匹配错误

**错误信息**: `the num_rows (N) of field (vector) is not equal to passed num_rows (M)`

**解决方案**:

1. 确保Azure OpenAI配置中设置了正确的维度限制
2. 检查Milvus的`EmbeddingDim`配置是否与实际嵌入维度匹配
3. 临时禁用文档分块功能进行调试: `EnableChunking: false`

### Milvus连接错误

**错误信息**: `database not found[database=xxx]`

**解决方案**: 使用空字符串连接默认数据库: `DBName: ""`

### GORM日志错误

**错误信息**: `nil pointer dereference`

**解决方案**: 使用`logger.Default.LogMode()`而不是`config.Logger.LogMode()`

### PostgreSQL向量数据库连接错误

**错误信息**: `relation "public.embeddings" does not exist`

**解决方案**: 确保PostgreSQL已安装并启用pgvector扩展：

```sql
CREATE EXTENSION IF NOT EXISTS vector;
```

## 📄 许可证

本项目采用 MIT 许可证。详情请参见 [LICENSE](LICENSE) 文件。

## 🤝 贡献

欢迎提交问题报告和功能请求。如果您想为项目做出贡献，请先开issue讨论您想要实现的更改。

---

**AGGO** - 让AI更智能，让开发更简单 🚀
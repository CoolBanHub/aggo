# AGGO - AI智能代理框架

AGGO是一个基于Go语言构建的智能AI代理框架，集成了对话AI、知识管理、记忆系统和工具调用等功能，基于CloudWeGo Eino框架开发。

## 🚀 核心特性

- **智能对话代理**: 基于React模式的AI代理，支持工具调用和多轮对话
- **知识库管理**: 双重存储架构，结合传统数据库和向量数据库实现高效的语义搜索
- **记忆系统**: 会话级记忆管理，支持长期记忆存储和智能摘要
- **工具集成**: 丰富的工具生态，包括知识推理、系统命令执行、数据库操作等
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

### 3. 存储集成测试

```bash
go run example/storage_vectordb_integration/main.go
```

### 4. GORM存储测试

```bash
go run example/gorm_storage_test/main.go
```

### 5. SSE流式响应示例

```bash
go run example/sse/main.go
```

## 💡 使用示例

### 创建知识库管理器

```go
package main

import (
	"context"
	"log"

	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/knowledge/storage"
	"github.com/CoolBanHub/aggo/knowledge/vectordb"
	"github.com/CoolBanHub/aggo/model"
)

func main() {
	ctx := context.Background()

	// 1. 创建嵌入模型
	em, err := model.NewEmbModel()
	if err != nil {
		log.Fatal(err)
	}

	// 2. 创建向量数据库 (Milvus)
	vectorDB, err := vectordb.NewMilvusVectorDB(vectordb.MilvusConfig{
		Address:        "127.0.0.1:19530",
		EmbeddingDim:   1024,
		DBName:         "", // 使用默认数据库
		CollectionName: "knowledge",
	})
	if err != nil {
		log.Fatal(err)
	}

	// 3. 创建存储层 (SQLite)
	storage, err := storage.NewSQLiteStorage("knowledge.db")
	if err != nil {
		log.Fatal(err)
	}

	// 4. 创建知识库管理器
	km, err := knowledge.NewKnowledgeManager(&knowledge.KnowledgeConfig{
		Storage:  storage,
		VectorDB: vectorDB,
		Em:       em,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 5. 加载文档
	docs := []knowledge.Document{
		{
			ID:      "doc1",
			Content: "Go语言是由Google开发的开源编程语言",
			Metadata: map[string]interface{}{
				"title": "Go语言介绍",
				"type":  "技术文档",
			},
		},
	}

	err = km.LoadDocuments(ctx, docs, knowledge.LoadOptions{
		EnableChunking: false,
		Upsert:         true,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 6. 搜索文档
	results, err := km.Search(ctx, "什么是Go语言", knowledge.SearchOptions{
		Limit:     5,
		Threshold: 0.7,
	})
	if err != nil {
		log.Fatal(err)
	}

	for _, result := range results {
		log.Printf("找到文档: %s (相似度: %.2f)", result.Document.Content, result.Score)
	}
}
```

### 创建智能代理

```go
import (
"github.com/CoolBanHub/aggo/agent"
"github.com/CoolBanHub/aggo/model"
)

func createAgent() (*agent.Agent, error) {
ctx := context.Background()

// 创建聊天模型
cm, err := model.NewChatModel()
if err != nil {
return nil, err
}

// 创建带知识库的代理
return agent.NewAgent(ctx, cm,
agent.WithKnowledgeManager(knowledgeManager),
agent.WithKnowledgeQueryConfig(&agent.KnowledgeQueryConfig{
MaxResults:  3,
Threshold:   0.7,
AlwaysQuery: false,
}),
agent.WithSystemPrompt("你是一个技术专家助手，能够搜索和分析相关技术信息。"),
)
}
```

## 🔧 配置说明

### 向量维度配置

系统统一使用**1024维度**向量：

```go
// Azure OpenAI嵌入配置
dimensions := 1024
config := &embopenai.EmbeddingConfig{
Model:      "text-embedding-3-large",
Dimensions: &dimensions, // 限制输出维度为1024
}

// Milvus配置
vectorConfig := vectordb.MilvusConfig{
EmbeddingDim: 1024, // 匹配嵌入维度
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
vectorDB, err := vectordb.NewMilvusVectorDB(vectordb.MilvusConfig{
	Address:        "127.0.0.1:19530",
	EmbeddingDim:   1024,
	DBName:         "", // 空字符串使用默认数据库
	CollectionName: "aggo",
})
```

**PostgreSQL向量数据库:**
```go
vectorDB, err := vectordb.NewPostgresVectorDB(vectordb.PostgresConfig{
	Host:         "localhost",
	Port:         5432,
	User:         "user",
	Password:     "password",
	DBName:       "vectordb",
	EmbeddingDim: 1024,
	TableName:    "embeddings",
})
```

## 🛠️ 开发指南

### 项目结构

```
aggo/
├── agent/              # AI代理系统
│   ├── agent.go           # 主代理实现
│   ├── knowledge_agent.go # 知识型代理
│   └── option.go          # 配置选项
├── knowledge/          # 知识管理系统
│   ├── manager.go         # 知识库管理器
│   ├── interfaces.go      # 接口定义
│   ├── storage/           # 存储层
│   ├── vectordb/          # 向量数据库
│   ├── readers/           # 文档读取器
│   └── chunking/          # 文档分块策略
├── memory/             # 记忆系统
├── model/              # AI模型封装
│   ├── chat.go            # 聊天模型
│   └── embedding.go       # 嵌入模型
├── tools/              # 工具集
│   ├── knowledge_tool.go      # 知识管理工具
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
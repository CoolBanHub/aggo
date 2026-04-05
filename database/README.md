# Database - 向量数据库（知识库存储层）

`database` 模块提供统一的向量存储和检索接口，是 AGGO 知识库系统的底层存储抽象。

## 架构

```
database.Database (接口)
    ├── indexer.Indexer    — 文档写入（向量化 + 存储）
    └── retriever.Retriever — 文档检索（向量相似度搜索）
```

所有实现同时满足 `indexer.Indexer` 和 `retriever.Retriever` 两个 Eino 接口，可以无缝接入 `tools/knowledge` 等上层工具。

## 支持的后端

| 后端 | 适用场景 | 依赖 |
|------|---------|------|
| **Milvus** | 生产环境、大规模数据 | Milvus >= 2.6 |
| **PostgreSQL + pgvector** | 开发/小规模、已有 PG 实例 | PostgreSQL >= 14 + pgvector |

## Milvus

基于 [eino-ext milvus2](https://github.com/cloudwego/eino-ext) 官方组件的薄封装，支持 COSINE 相似度搜索和 ScoreThreshold 过滤。

### 创建实例

```go
import (
    "context"

    "github.com/CoolBanHub/aggo/database/milvus"
    "github.com/milvus-io/milvus/client/v2/milvusclient"
)

client, _ := milvusclient.New(ctx, &milvusclient.ClientConfig{
    Address: "127.0.0.1:19530",
})

db, _ := milvus.NewMilvus(ctx, milvus.MilvusConfig{
    Client:         client,
    CollectionName: "knowledge_vectors",  // 可选，默认 aggo_knowledge_vectors
    EmbeddingDim:   1024,
    Embedding:      embeddingModel,
})
```

### 文档存储

```go
ids, _ := db.Store(ctx, []*schema.Document{
    {ID: "doc1", Content: "Go语言介绍...", MetaData: map[string]any{"source": "tech"}},
})
```

`Store` 内部会调用 `Embedding.EmbedStrings` 批量向量化，然后 Upsert 到 Milvus。

### 文档检索

```go
docs, _ := db.Retrieve(ctx, "什么是Go语言",
    retriever.WithTopK(5),
    retriever.WithScoreThreshold(0.3),
)
```

`Retrieve` 内部使用 `search_mode.Approximate` (ANN) + COSINE 度量，并按 `ScoreThreshold` 过滤低分结果。

### 配置说明

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `Client` | `*milvusclient.Client` | 是 | - | Milvus 客户端 |
| `CollectionName` | `string` | 否 | `aggo_knowledge_vectors` | 集合名称 |
| `EmbeddingDim` | `int` | 是 | - | 向量维度 |
| `Embedding` | `embedding.Embedder` | 是 | - | 嵌入模型 |

### 内部实现

Milvus 实现是对 eino-ext 官方组件的薄封装：

- **Indexer**: `eino-ext/components/indexer/milvus2.Indexer` — 负责文档向量化、集合初始化、Upsert
- **Retriever**: `eino-ext/components/retriever/milvus2.Retriever` + `search_mode.Approximate` — 负责 ANN 搜索
- **ScoreThreshold 过滤**: 在官方 retriever 返回结果之上，按 `ScoreThreshold` 过滤低分文档

使用官方组件带来的优势：
- 批量 Embedding（非逐文档调用）
- 正确的 Upsert ID 返回
- 集合状态检查、索引创建 Await 等最佳实践
- 未来可按需扩展 Sparse Vector、Hybrid Search、BM25 等高级模式

## PostgreSQL + pgvector

基于 GORM + pgvector 的轻量实现，适合开发环境或小规模场景。

### 创建实例

```go
import (
    "github.com/CoolBanHub/aggo/database/postgres"
    "gorm.io/gorm"
)

db, _ := postgres.NewPostgres(postgres.PostgresConfig{
    Client:          gormDB,
    CollectionName:  "knowledge_vectors",
    VectorDimension: 1024,
    Embedding:       embeddingModel,
})
```

### 配置说明

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `Client` | `*gorm.DB` | 是 | - | GORM 数据库实例 |
| `CollectionName` | `string` | 否 | `aggo_knowledge_vectors` | 表名 |
| `VectorDimension` | `int` | 否 | `1536` | 向量维度 |
| `Embedding` | `embedding.Embedder` | 是 | - | 嵌入模型 |

## 与知识库工具集成

`database.Database` 实例可直接传入 `tools.GetKnowledgeTools`：

```go
knowledgeTools := tools.GetKnowledgeTools(db, db, &retriever.Options{
    TopK:           utils.ValueToPtr(10),
    ScoreThreshold: utils.ValueToPtr(0.2),
})
```

也可以配合 `eino/flow/retriever/router` 实现多检索器路由。

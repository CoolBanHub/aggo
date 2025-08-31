# Knowledge Storage

基于 GORM 的通用知识库存储层，专注于文档的基本信息存储，支持 MySQL、PostgreSQL 和 SQLite。

## 架构设计

**职责分离**：
- **Storage 层**: 专门存储文档的基本信息（ID、Content、Metadata、时间戳）
- **VectorDB 层**: 专门处理向量化数据（ID、Content、Vector），用于语义搜索
- **KnowledgeManager**: 协调 Storage 和 VectorDB，提供统一的知识库管理接口

## 特性

- 统一的存储接口，支持多种数据库
- 基于 GORM，提供强大的 ORM 功能
- 自动数据库迁移
- 连接池管理
- 批量操作支持
- JSON 元数据存储
- 与 VectorDB 分离设计，各司其职

## 支持的数据库

- **SQLite**: 适合开发环境和小型应用
- **MySQL**: 适合生产环境，高性能和可扩展性
- **PostgreSQL**: 适合复杂查询和高级功能需求

## 快速开始

### 1. SQLite 存储

```go
package main

import (
    "context"
    "log"
    
    "github.com/CoolBanHub/aggo/knowledge"
    "github.com/CoolBanHub/aggo/knowledge/storage"
)

func main() {
    ctx := context.Background()
    
    // 创建 SQLite 存储
    store, err := storage.NewSQLiteStorage("knowledge.db")
    if err != nil {
        log.Fatal(err)
    }
    defer store.Close()
    
    // 保存文档（注意：Storage 层不处理向量数据）
    doc := &knowledge.Document{
        ID:      "doc1",
        Content: "Go语言是一门优秀的编程语言",
        Metadata: map[string]interface{}{
            "title": "Go语言介绍",
            "author": "Google",
        },
        // Vector 字段由 VectorDB 层管理，Storage 层不涉及
    }
    
    if err := store.SaveDocument(ctx, doc); err != nil {
        log.Fatal(err)
    }
    
    // 获取文档
    retrievedDoc, err := store.GetDocument(ctx, "doc1")
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("文档内容: %s", retrievedDoc.Content)
}
```

### 2. MySQL 存储

```go
// 创建 MySQL 存储
store, err := storage.NewMySQLStorage("localhost", 3306, "knowledge_db", "username", "password")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

### 3. PostgreSQL 存储

```go
// 创建 PostgreSQL 存储
store, err := storage.NewPostgresSQLStorage("localhost", 5432, "knowledge_db", "username", "password")
if err != nil {
log.Fatal(err)
}
defer store.Close()

```

## 高级配置

### 自定义配置

```go
import "github.com/CoolBanHub/aggo/knowledge/storage"

// 创建自定义配置
config := &storage.Config{
    Type:     storage.MySQL,
    Host:     "localhost",
    Port:     3306,
    Database: "knowledge_db",
    Username: "user",
    Password: "password",
    
    // 连接池配置
    MaxOpenConns: 50,
    MaxIdleConns: 10,
    
    // 日志配置
    LogLevel: 4, // Info 级别
}

// 创建存储实例
store, err := storage.NewGormStorage(config)
if err != nil {
    log.Fatal(err)
}
```

### 使用选项模式

```go
config := storage.NewMySQLConfig("localhost", 3306, "knowledge_db", "user", "password")

options := &storage.StorageOptions{
    MaxOpenConns: 100,
    MaxIdleConns: 20,
    LogLevel:     3, // Warn 级别
}

store, err := storage.NewStorageWithOptions(config, options)
if err != nil {
    log.Fatal(err)
}
```

## 基本操作

### 文档操作

```go
ctx := context.Background()

// 保存文档（Storage 层只处理基本信息）
doc := &knowledge.Document{
    ID:      "unique_id",
    Content: "文档内容",
    Metadata: map[string]interface{}{
        "title":  "标题",
        "author": "作者",
        "tags":   []string{"tag1", "tag2"},
    },
    // Vector 数据交给 VectorDB 层处理
}

err := store.SaveDocument(ctx, doc)

// 获取文档
doc, err := store.GetDocument(ctx, "unique_id")

// 更新文档
doc.Content = "更新的内容"
err = store.UpdateDocument(ctx, doc)

// 删除文档
err = store.DeleteDocument(ctx, "unique_id")
```

### 批量操作

```go
// 批量保存文档
docs := []*knowledge.Document{
    {ID: "doc1", Content: "内容1"},
    {ID: "doc2", Content: "内容2"},
    {ID: "doc3", Content: "内容3"},
}

// 批量大小为 100
err := store.BatchSaveDocuments(ctx, docs, 100)
```

### 查询操作

```go
// 列出文档（分页）
docs, err := store.ListDocuments(ctx, 10, 0) // limit=10, offset=0

// 搜索文档
docs, err := store.SearchDocuments(ctx, "搜索关键词", 5)

// 获取文档总数
count, err := store.Count(ctx)
```

## 数据库配置示例

### MySQL 配置

```go
config := storage.NewMySQLConfig("localhost", 3306, "knowledge_db", "user", "password")
config.MaxOpenConns = 50
config.MaxIdleConns = 10
config.LogLevel = 3
```

### PostgreSQL 配置

```go
config := storage.NewPostgreSQLConfig("localhost", 5432, "knowledge_db", "user", "password")
config.MaxOpenConns = 50
config.MaxIdleConns = 10
config.LogLevel = 3
```

### SQLite 配置

```go
config := storage.NewSQLiteConfig("/path/to/database.db")
config.LogLevel = 4 // 开启详细日志
```

## 日志级别

- `1`: Silent - 无日志输出
- `2`: Error - 仅错误日志
- `3`: Warn - 警告和错误日志
- `4`: Info - 所有日志（包括 SQL 查询）

## 架构集成

### 与 VectorDB 的协作

```go
// 典型的使用流程
func SaveDocumentWithVector(ctx context.Context, doc *knowledge.Document, 
    storage knowledge.KnowledgeStorage, vectorDB knowledge.VectorDB) error {
    
    // 1. 在 Storage 层保存文档基本信息
    if err := storage.SaveDocument(ctx, doc); err != nil {
        return err
    }
    
    // 2. 在 VectorDB 层保存向量化数据（需要先生成向量）
    if err := vectorDB.Insert(ctx, []knowledge.Document{*doc}); err != nil {
        // 如果向量存储失败，可以考虑回滚 Storage 操作
        storage.DeleteDocument(ctx, doc.ID)
        return err
    }
    
    return nil
}
```

### KnowledgeManager 集成

通常，你不会直接使用 Storage 层，而是通过 `KnowledgeManager` 来协调 Storage 和 VectorDB：

```go
// KnowledgeManager 会自动协调两个存储层
manager, err := knowledge.NewKnowledgeManager(&knowledge.KnowledgeConfig{
    Storage:  storage.NewSQLiteStorage("docs.db"),  // Storage 层
    VectorDB: vectordb.NewMilvusVectorDB(config),   // VectorDB 层
})

// 保存文档时，Manager 会同时操作两个存储层
err = manager.SaveDocument(ctx, doc)
```

## 注意事项

1. **数据库创建**: 确保目标数据库已创建（SQLite 除外，会自动创建文件）
2. **权限设置**: 确保数据库用户有足够权限创建表和执行操作
3. **连接池**: 根据应用负载调整 `MaxOpenConns` 和 `MaxIdleConns`
4. **JSON 序列化**: 元数据会被序列化为 JSON 存储
5. **软删除**: 使用 GORM 的软删除功能，数据不会物理删除
6. **职责分离**: Storage 不处理向量数据，向量相关操作交给 VectorDB 层

## 性能优化建议

1. **索引**: 对常用查询字段添加数据库索引
2. **批量操作**: 对于大量数据使用 `BatchSaveDocuments`
3. **连接池**: 合理配置连接池大小
4. **分页**: 使用 `ListDocuments` 进行分页查询
5. **文本搜索**: Storage 层的 `SearchDocuments` 仅提供基本文本搜索，语义搜索请使用 VectorDB
6. **数据一致性**: 在 KnowledgeManager 层确保 Storage 和 VectorDB 的数据一致性

## 故障排除

### 常见错误

1. **连接失败**: 检查数据库服务是否运行，连接参数是否正确
2. **权限不足**: 确保数据库用户有建表和数据操作权限
3. **JSON 序列化错误**: 检查元数据中是否有不可序列化的类型
4. **文档 ID 冲突**: 确保文档 ID 唯一性

### 调试方法

```go
// 开启详细日志
config.LogLevel = 4

// 获取底层 GORM 实例进行调试
gormStore := store.(*storage.GormStorage)
db := gormStore.GetDB()
```

## 扩展

如需添加新的数据库支持或自定义功能，可以：

1. 在 `config.go` 中添加新的数据库类型
2. 在 `gorm_storage.go` 中添加对应的处理逻辑
3. 实现 `knowledge.KnowledgeStorage` 接口

## 依赖

```go
require (
    gorm.io/gorm v1.25.0
    gorm.io/driver/mysql v1.5.0
    gorm.io/driver/postgres v1.5.0
    gorm.io/driver/sqlite v1.5.0
)
```
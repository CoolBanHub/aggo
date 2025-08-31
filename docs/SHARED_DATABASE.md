# 共享数据库连接使用指南

在AGGO框架中，knowledge模块和memory模块都需要使用关系型数据库存储数据。为了优化资源利用和减少数据库连接数，现在支持两个模块共享同一个GORM数据库实例。

## 🎯 优势

### 资源优化
- **减少连接数**: 多个模块共用一个连接池，避免重复创建数据库连接
- **降低内存占用**: 共享GORM实例和连接池配置
- **简化配置管理**: 统一的数据库配置，便于维护

### 性能提升
- **连接池复用**: 更高效的连接池利用率
- **事务一致性**: 可以在同一个事务中操作多个模块的数据
- **减少连接开销**: 避免频繁创建和销毁数据库连接

## 🛠️ 实现方式

### Knowledge模块新增函数

```go
// NewGormStorageWithDB 使用现有的 GORM 实例创建存储实例
func NewGormStorageWithDB(db *gorm.DB, config *Config) (*GormStorage, error)

// NewStorageWithSharedDB 便捷函数
func NewStorageWithSharedDB(db *gorm.DB, config *Config) (knowledge.KnowledgeStorage, error)
```

### Memory模块新增函数

```go
// NewSQLStoreWithDB 使用现有的 GORM 实例创建SQL存储实例
func NewSQLStoreWithDB(db *gorm.DB, dialect SQLDialect) (*SQLStore, error)
```

## 📖 使用示例

### 1. SQLite共享示例

```go
package main

import (
    "context"
    "log"
    
    "github.com/CoolBanHub/aggo/knowledge/storage"
    memorystorage "github.com/CoolBanHub/aggo/memory/storage"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

func main() {
    ctx := context.Background()
    
    // 1. 创建共享的GORM数据库实例
    sharedDB, err := gorm.Open(sqlite.Open("shared_aggo.db"), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info),
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 2. 创建knowledge存储（使用共享DB）
    knowledgeStorage, err := storage.NewStorageWithSharedDB(sharedDB, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer knowledgeStorage.Close()
    
    // 3. 创建memory存储（使用共享DB）
    memoryStorage, err := memorystorage.NewSQLStoreWithDB(sharedDB, memorystorage.DialectSQLite)
    if err != nil {
        log.Fatal(err)
    }
    defer memoryStorage.Close()
    
    log.Println("共享数据库连接创建成功！")
    
    // 现在两个模块使用同一个数据库连接
}
```

### 2. MySQL共享示例

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/CoolBanHub/aggo/knowledge/storage"
    memorystorage "github.com/CoolBanHub/aggo/memory/storage"
    "gorm.io/driver/mysql"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

func main() {
    ctx := context.Background()
    
    // 1. 创建共享的GORM数据库实例
    dsn := "user:password@tcp(localhost:3306)/aggo?charset=utf8mb4&parseTime=True&loc=Local"
    sharedDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info),
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 配置连接池
    sqlDB, err := sharedDB.DB()
    if err != nil {
        log.Fatal(err)
    }
    sqlDB.SetMaxIdleConns(10)
    sqlDB.SetMaxOpenConns(50)
    sqlDB.SetConnMaxLifetime(time.Hour)
    
    // 2. 创建knowledge存储（使用共享DB和自定义配置）
    knowledgeConfig := &storage.Config{
        Type:         storage.MySQL,
        MaxOpenConns: 50,
        MaxIdleConns: 10,
        LogLevel:     3, // Warn
    }
    knowledgeStorage, err := storage.NewStorageWithSharedDB(sharedDB, knowledgeConfig)
    if err != nil {
        log.Fatal(err)
    }
    defer knowledgeStorage.Close()
    
    // 3. 创建memory存储（使用共享DB）
    memoryStorage, err := memorystorage.NewSQLStoreWithDB(sharedDB, memorystorage.DialectMySQL)
    if err != nil {
        log.Fatal(err)
    }
    defer memoryStorage.Close()
    
    log.Println("MySQL共享数据库连接创建成功！")
}
```

### 3. 完整的应用集成示例

```go
package main

import (
    "context"
    "log"
    
    "github.com/CoolBanHub/aggo/agent"
    "github.com/CoolBanHub/aggo/knowledge"
    "github.com/CoolBanHub/aggo/knowledge/storage"
    "github.com/CoolBanHub/aggo/knowledge/vectordb"
    "github.com/CoolBanHub/aggo/memory"
    memorystorage "github.com/CoolBanHub/aggo/memory/storage"
    "github.com/CoolBanHub/aggo/model"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

func main() {
    ctx := context.Background()
    
    // 1. 创建共享数据库
    sharedDB, err := gorm.Open(sqlite.Open("app.db"), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Warn),
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 2. 创建存储层
    knowledgeStorage, err := storage.NewStorageWithSharedDB(sharedDB, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer knowledgeStorage.Close()
    
    memoryStorage, err := memorystorage.NewSQLStoreWithDB(sharedDB, memorystorage.DialectSQLite)
    if err != nil {
        log.Fatal(err)
    }
    defer memoryStorage.Close()
    
    // 3. 创建其他组件
    em, err := model.NewEmbModel()
    if err != nil {
        log.Fatal(err)
    }
    
    vectorDB, err := vectordb.NewMilvusVectorDB(vectordb.MilvusConfig{
        Address:        "127.0.0.1:19530",
        EmbeddingDim:   1024,
        DBName:         "",
        CollectionName: "app_knowledge",
    })
    if err != nil {
        log.Printf("Milvus连接失败，使用Mock: %v", err)
        vectorDB = vectordb.NewMockVectorDB()
    }
    
    // 4. 创建管理器
    knowledgeManager, err := knowledge.NewKnowledgeManager(&knowledge.KnowledgeConfig{
        Storage:  knowledgeStorage,
        VectorDB: vectorDB,
        Em:       em,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    memoryManager, err := memory.NewMemoryManager(memoryStorage, &memory.MemoryConfig{
        MaxUserMemories:    100,
        MaxSessionMessages: 50,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 5. 创建AI代理
    chatModel, err := model.NewChatModel()
    if err != nil {
        log.Fatal(err)
    }
    
    aiAgent, err := agent.NewAgent(ctx, chatModel,
        agent.WithKnowledgeManager(knowledgeManager),
        agent.WithMemoryManager(memoryManager),
        agent.WithUserID("user123"),
        agent.WithSessionID("session456"),
        agent.WithSystemPrompt("我是一个使用共享数据库的AI助手"),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    log.Println("应用初始化完成，所有组件使用共享数据库连接！")
    
    // 现在可以使用aiAgent进行对话
    // knowledge和memory模块共享同一个数据库连接
}
```

## 🔧 配置说明

### Knowledge模块配置

使用共享数据库时，知识库存储的配置主要用于设置业务逻辑相关参数：

```go
config := &storage.Config{
    Type:         storage.MySQL, // 数据库类型
    MaxOpenConns: 50,           // 这些连接池设置会被忽略
    MaxIdleConns: 10,           // 因为使用的是共享连接
    LogLevel:     3,            // 日志级别
}
```

### Memory模块配置

Memory模块需要指定数据库方言类型：

```go
// 支持的方言类型
memorystorage.DialectMySQL      // "mysql"
memorystorage.DialectPostgreSQL // "postgres" 
memorystorage.DialectSQLite     // "sqlite"
```

### 连接池配置

连接池配置应该在创建共享GORM实例时进行：

```go
// 创建共享DB时配置连接池
sharedDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Info),
})

// 配置连接池参数
sqlDB, _ := sharedDB.DB()
sqlDB.SetMaxIdleConns(10)      // 空闲连接数
sqlDB.SetMaxOpenConns(50)      // 最大连接数
sqlDB.SetConnMaxLifetime(time.Hour) // 连接生存时间
```

## 📊 性能对比

### 传统方式 vs 共享方式

| 对比项 | 传统方式 | 共享方式 |
|--------|----------|----------|
| 数据库连接数 | Knowledge: 25 + Memory: 25 = 50 | 共享: 50 |
| 内存占用 | 两套连接池 + 配置 | 一套连接池 + 配置 |
| 配置复杂度 | 需要两套配置 | 统一配置 |
| 事务支持 | 跨模块事务复杂 | 支持统一事务 |
| 资源利用率 | 可能存在空闲连接浪费 | 更高效的连接复用 |

## ⚠️ 注意事项

### 1. 数据库迁移

两个模块都会自动执行数据库迁移，确保表结构正确创建：

- Knowledge模块会创建 `document_models` 表
- Memory模块会创建 `user_memory_models`、`session_summary_models`、`conversation_message_models` 表

### 2. 事务处理

如果需要跨模块的事务操作，可以这样实现：

```go
err := sharedDB.Transaction(func(tx *gorm.DB) error {
    // 在事务中操作knowledge存储
    knowledgeStorageTx, _ := storage.NewStorageWithSharedDB(tx, nil)
    
    // 在事务中操作memory存储  
    memoryStorageTx, _ := memorystorage.NewSQLStoreWithDB(tx, memorystorage.DialectMySQL)
    
    // 执行跨模块操作
    // ...
    
    return nil
})
```

### 3. 错误处理

确保正确处理数据库连接错误：

```go
sharedDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
if err != nil {
    log.Fatalf("数据库连接失败: %v", err)
}

// 测试连接
sqlDB, err := sharedDB.DB()
if err != nil {
    log.Fatalf("获取底层数据库失败: %v", err)
}

if err := sqlDB.Ping(); err != nil {
    log.Fatalf("数据库连接测试失败: %v", err)
}
```

### 4. 关闭连接

使用共享数据库时，只需要关闭一次数据库连接：

```go
defer func() {
    if sqlDB, err := sharedDB.DB(); err == nil {
        sqlDB.Close()
    }
}()

// 或者在各个存储的Close()方法中会处理，但不会真正关闭共享连接
defer knowledgeStorage.Close()
defer memoryStorage.Close()
```

## 🚀 运行示例

运行完整的共享数据库示例：

```bash
cd example/shared_database
go run main.go
```

这个示例展示了：
1. 如何创建共享数据库连接
2. 如何配置knowledge和memory存储使用共享连接
3. 如何集成到完整的AI代理应用中
4. 性能优化的实际效果

通过使用共享数据库连接，你可以显著提高应用的资源利用效率，简化配置管理，同时保持各模块的功能完整性。
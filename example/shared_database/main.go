package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/knowledge/readers"
	"github.com/CoolBanHub/aggo/knowledge/storage"
	"github.com/CoolBanHub/aggo/knowledge/vectordb"
	"github.com/CoolBanHub/aggo/memory"
	memorystorage "github.com/CoolBanHub/aggo/memory/storage"
	"github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	schemaGrom "gorm.io/gorm/schema"
)

func main() {
	ctx := context.Background()

	// ===== 方式一：使用SQLite共享数据库实例 =====
	log.Println("=== 使用SQLite共享数据库示例 ===")
	if err := sqliteSharedExample(ctx); err != nil {
		log.Printf("SQLite共享示例失败: %v", err)
	}

	// ===== 方式二：使用MySQL共享数据库实例 =====
	log.Println("\n=== 使用MySQL共享数据库示例 ===")
	if err := mysqlSharedExample(ctx); err != nil {
		log.Printf("MySQL共享示例失败: %v", err)
	}
}

// sqliteSharedExample SQLite共享数据库示例
func sqliteSharedExample(ctx context.Context) error {
	// 1. 创建共享的GORM数据库实例
	gormSql, err := NewMysqlGrom("mysql://root:123456@localhost:3306/aggo", logger.Silent)
	if err != nil {
		return err
	}
	// 2. 使用共享数据库创建knowledge存储
	knowledgeStorage, err := storage.NewGormStorage(gormSql)
	if err != nil {
		return err
	}
	defer knowledgeStorage.Close()

	// 3. 使用共享数据库创建memory存储
	memoryStorage, err := memorystorage.NewGormStorage(gormSql)
	if err != nil {
		return err
	}
	defer memoryStorage.Close()

	// 4. 创建其他组件并演示使用
	return demonstrateSharedUsage(ctx, knowledgeStorage, memoryStorage)
}

// mysqlSharedExample MySQL共享数据库示例
func mysqlSharedExample(ctx context.Context) error {
	gormSql, err := NewMysqlGrom("mysql://root:123456@localhost:3306/aggo", logger.Silent)
	if err != nil {
		return err
	}

	knowledgeStorage, err := storage.NewGormStorage(gormSql)
	if err != nil {
		return err
	}
	defer knowledgeStorage.Close()

	// 3. 使用共享数据库创建memory存储
	memoryStorage, err := memorystorage.NewGormStorage(gormSql)
	if err != nil {
		return err
	}
	defer memoryStorage.Close()

	// 4. 创建其他组件并演示使用
	return demonstrateSharedUsage(ctx, knowledgeStorage, memoryStorage)
}

// demonstrateSharedUsage 演示共享数据库的使用
func demonstrateSharedUsage(ctx context.Context, knowledgeStorage knowledge.KnowledgeStorage, memoryStorage memory.MemoryStorage) error {
	log.Println("开始演示共享数据库使用...")

	// 1. 创建嵌入模型
	em, err := model.NewEmbModel()
	if err != nil {
		return err
	}

	// 2. 创建向量数据库（这里仍然独立，因为是不同的存储类型）

	client, err := milvusclient.New(context.Background(), &milvusclient.ClientConfig{
		Address: "127.0.0.1:19530",
		DBName:  "",
	})
	if err != nil {
		return err
	}

	var vectorDB knowledge.VectorDB
	milvusVectorDB, err := vectordb.NewMilvusVectorDB(vectordb.MilvusConfig{
		Client:         client,
		EmbeddingDim:   1024,
		CollectionName: "shared_example",
	})
	if err != nil {
		log.Printf("Milvus连接失败，使用Mock向量数据库: %v", err)
		vectorDB = vectordb.NewMockVectorDB()
	} else {
		vectorDB = milvusVectorDB
	}

	// 3. 创建知识库管理器（使用共享的knowledge存储）
	knowledgeManager, err := knowledge.NewKnowledgeManager(&knowledge.KnowledgeConfig{
		Storage:  knowledgeStorage,
		VectorDB: vectorDB,
		Em:       em,
	})
	if err != nil {
		return err
	}

	// 4. 创建聊天模型（记忆管理器需要）
	chatModel, err := model.NewChatModel()
	if err != nil {
		log.Printf("聊天模型创建失败，跳过记忆管理器: %v", err)
		chatModel = nil
	}

	// 5. 创建记忆管理器（使用共享的memory存储）
	var memoryManager *memory.MemoryManager
	if chatModel != nil {
		memoryManager = memory.NewMemoryManager(chatModel, memoryStorage, &memory.MemoryConfig{
			EnableUserMemories:   true,
			EnableSessionSummary: false,
			MemoryLimit:          100,
		})
	}

	// 6. 加载一些测试文档到知识库
	docs := []knowledge.Document{
		{
			ID:      "shared_doc_1",
			Content: "这是一个使用共享数据库存储的测试文档",
			Metadata: map[string]interface{}{
				"title":  "共享存储测试",
				"source": "示例代码",
			},
		},
		{
			ID:      "shared_doc_2",
			Content: "共享数据库可以减少连接数，提高资源利用率",
			Metadata: map[string]interface{}{
				"title":  "性能优化",
				"source": "最佳实践",
			},
		},
	}

	reader := readers.NewInMemoryReader(docs)
	documents, err := reader.ReadDocuments(ctx)
	if err != nil {
		return err
	}

	err = knowledgeManager.LoadDocuments(ctx, documents, knowledge.LoadOptions{
		EnableChunking: false,
		Upsert:         true,
	})
	if err != nil {
		return err
	}
	log.Printf("成功加载 %d 个文档到共享知识库", len(documents))

	// 7. 测试知识搜索
	results, err := knowledgeManager.Search(ctx, "共享数据库", knowledge.SearchOptions{
		Limit:     2,
		Threshold: 0.1, // 降低阈值以便测试
	})
	if err != nil {
		return err
	}
	log.Printf("搜索到 %d 个相关文档", len(results))

	// 8. 测试记忆功能
	userID := "test_user_shared"
	sessionID := "session_shared_db"

	// 保存一些测试消息
	messages := []*memory.ConversationMessage{
		{
			ID:        "msg1",
			SessionID: sessionID,
			UserID:    userID,
			Role:      "user",
			Content:   "测试共享数据库的记忆功能",
			CreatedAt: time.Now(),
		},
		{
			ID:        "msg2",
			SessionID: sessionID,
			UserID:    userID,
			Role:      "assistant",
			Content:   "共享数据库记忆功能正常工作",
			CreatedAt: time.Now(),
		},
	}

	for _, msg := range messages {
		if err := memoryStorage.SaveMessage(ctx, msg); err != nil {
			return err
		}
	}

	// 保存用户记忆
	userMemory := &memory.UserMemory{
		ID:        "memory_shared_1",
		UserID:    userID,
		Memory:    "用户喜欢使用共享数据库优化性能",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := memoryStorage.SaveUserMemory(ctx, userMemory); err != nil {
		return err
	}

	log.Printf("成功保存 %d 条消息和 1 条用户记忆", len(messages))

	// 9. 查询记忆
	retrievedMessages, err := memoryStorage.GetMessages(ctx, sessionID, userID, 10)
	if err != nil {
		return err
	}
	log.Printf("查询到 %d 条历史消息", len(retrievedMessages))

	retrievedMemories, err := memoryStorage.GetUserMemories(ctx, userID, 10, memory.RetrievalLastN)
	if err != nil {
		return err
	}
	log.Printf("查询到 %d 条用户记忆", len(retrievedMemories))

	// 10. 创建使用共享存储的代理（示例）
	if chatModel == nil {
		log.Printf("聊天模型不可用，跳过代理测试")
		return nil
	}

	aiAgent, err := agent.NewAgent(ctx, chatModel,
		agent.WithKnowledgeManager(knowledgeManager),
		agent.WithMemoryManager(memoryManager),
		agent.WithUserID(userID),
		agent.WithSessionID(sessionID),
		agent.WithSystemPrompt("我是一个使用共享数据库的AI助手"),
	)
	if err != nil {
		log.Printf("代理创建失败，跳过代理测试: %v", err)
		return nil
	}

	// 测试对话
	response, err := aiAgent.Generate(ctx, []*schema.Message{
		schema.UserMessage("你能告诉我关于共享数据库的信息吗？"),
	})
	if err != nil {
		log.Printf("代理对话失败: %v", err)
		return nil
	}

	log.Printf("AI回复: %s", response.Content)

	log.Println("共享数据库示例完成！")
	return nil
}

func NewMysqlGrom(source string, logLevel logger.LogLevel) (*gorm.DB, error) {
	if !strings.Contains(source, "parseTime") {
		source += "?charset=utf8mb4&parseTime=True&loc=Local"
	}
	gdb, err := gorm.Open(mysql.Open(source), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		NamingStrategy: schemaGrom.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		panic("数据库连接失败: " + err.Error())
	}

	// 配置GORM日志
	var gormLogger logger.Interface
	if logLevel > 0 {
		gormLogger = logger.Default.LogMode(logLevel)
	} else {
		gormLogger = logger.Default.LogMode(logger.Silent)
	}

	gdb.Logger = gormLogger

	return gdb, nil
}

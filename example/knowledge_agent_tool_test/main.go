package main

import (
	"context"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/database"
	"github.com/CoolBanHub/aggo/database/milvus"
	postgres2 "github.com/CoolBanHub/aggo/database/postgres"
	"github.com/CoolBanHub/aggo/memory"
	storage2 "github.com/CoolBanHub/aggo/memory/storage"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/flow/retriever/router"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	schemaGrom "gorm.io/gorm/schema"
)

func main() {
	ctx := context.Background()

	// 1. 创建聊天模型
	cm, err := model.NewChatModel(model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("gpt-5-mini"),
	)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
		return
	}

	em, err := model.NewEmbModel(model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("text-embedding-3-large"),
	)
	if err != nil {
		log.Fatalf("创建嵌入模型失败: %v", err)
		return
	}

	gormSql, err := NewPostgresGorm("host=localhost user=postgres password=postgres dbname=test port=5432 sslmode=disable", logger.Silent)
	if err != nil {
		log.Fatalf("创建gorm失败: %v", err)
		return
	}

	databaseDB := getPostgresDB(gormSql, em)
	//databaseDB := getMilvusDB(em)

	log.Println("开始添加数据")

	// 4. 加载示例文档到知识库（模拟一些技术文档）
	docs := []*schema.Document{
		{
			ID:      utils.GetUUIDNoDash(),
			Content: "Go语言是由Google开发的开源编程语言，以其简洁性和高效性著称。它特别适用于系统编程、网络服务和云计算应用。",
			MetaData: map[string]interface{}{
				"title":  "Go语言介绍",
				"source": "技术文档",
				"type":   "编程语言",
			},
		},
		{
			ID:      utils.GetUUIDNoDash(),
			Content: "微服务架构是一种将单一应用程序开发为一套小服务的方法，每个服务运行在自己的进程中，服务间通过HTTP API进行通信。",
			MetaData: map[string]interface{}{
				"title":  "微服务架构",
				"source": "架构文档",
				"type":   "系统架构",
			},
		},
		{
			ID:      utils.GetUUIDNoDash(),
			Content: "Docker是一个开源的应用容器引擎，让开发者可以打包他们的应用以及依赖包到一个可移植的镜像中，然后发布到任何Linux或Windows机器上。",
			MetaData: map[string]interface{}{
				"title":  "Docker容器技术",
				"source": "技术文档",
				"type":   "容器技术",
			},
		},
	}
	if false {
		_, err = databaseDB.Store(ctx, docs)
		if err != nil {
			log.Fatalf("加载文档到知识库失败: %v", err)
		}
		log.Println("知识库初始化完成，已加载示例文档")
	}
	s, err := storage2.NewGormStorage(gormSql)
	if err != nil {
		log.Fatalf("new sql store fail,err:%s", err)
		return
	}
	memoryManager, err := memory.NewMemoryManager(cm, s, &memory.MemoryConfig{
		EnableSessionSummary: false,
		EnableUserMemories:   false,
		MemoryLimit:          8,
		Retrieval:            memory.RetrievalLastN,
		AsyncProcessing:      true,
	})
	if err != nil {
		log.Fatalf("new manager fail,err:%s", err)
		return
	}
	defer memoryManager.Close()
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
		return
	}
	// 5. 创建主 Agent，将 KnowledgeAgent 作为工具
	mainAgent, err := agent.NewAgent(ctx, cm,
		agent.WithMemoryManager(memoryManager),
		agent.WithKnowledgeQueryConfig(&agent.KnowledgeQueryConfig{
			AlwaysQuery: true,
		}),
		agent.WithRetriever(routerRetriever),
		agent.WithSystemPrompt("你是一个技术专家助手。当用户询问技术问题时，你应该使用 knowledge_reason 工具来搜索和分析相关信息，然后提供准确的回答。"))

	if err != nil {
		log.Fatalf("创建主Agent失败: %v", err)
	}

	// 6. 测试对话 - 询问技术问题
	testQuestions := []string{
		"什么是Go语言？它有什么特点？",
		"能解释一下微服务架构吗？",
		"Docker是什么？有什么用途？",
		"Go语言适合用来开发哪些类型的应用？",
	}
	userID := utils.GetUUIDNoDash()
	sessionId := utils.GetUUIDNoDash()
	for i, question := range testQuestions {
		log.Printf("\n=== 测试问题 %d ===", i+1)
		log.Printf("用户: %s", question)

		response, err := mainAgent.Generate(ctx, []*schema.Message{
			schema.UserMessage(question),
		}, agent.WithChatUserID(userID),
			agent.WithChatSessionID(sessionId),
		)
		if err != nil {
			log.Printf("生成回答失败: %v", err)
			continue
		}

		log.Printf("AI助手: %s", response.Content)
	}
}

func getMilvusDB(em embedding.Embedder) database.Database {
	client, err := milvusclient.New(context.Background(), &milvusclient.ClientConfig{
		Address: "127.0.0.1:19530",
		DBName:  "",
	})
	if err != nil {
		return nil
	}
	milvusDB, err := milvus.NewMilvus(milvus.MilvusConfig{
		Client:         client,
		CollectionName: "aggo_knowledge_vectors",
		EmbeddingDim:   1024,
		Embedding:      em,
	})
	if err != nil {
		log.Fatalf("创建数据库失败: %v", err)
		return nil
	}
	return milvusDB
}

func getPostgresDB(gormSql *gorm.DB, em embedding.Embedder) database.Database {
	postgresDB, err := postgres2.NewPostgres(postgres2.PostgresConfig{
		Client:          gormSql,
		CollectionName:  "aggo_knowledge_vectors",
		VectorDimension: 1024,
		Embedding:       em,
	})
	if err != nil {
		log.Fatalf("创建数据库失败: %v", err)
		return nil
	}
	return postgresDB
}

func NewPostgresGorm(source string, logLevel logger.LogLevel) (*gorm.DB, error) {
	// PostgreSQL 不需要像 MySQL 那样的 parseTime 参数
	// 但你可能需要确保连接字符串格式正确

	gdb, err := gorm.Open(postgres.Open(source), &gorm.Config{
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

package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/knowledge/readers"
	"github.com/CoolBanHub/aggo/knowledge/storage"
	"github.com/CoolBanHub/aggo/knowledge/vectordb"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	schemaGrom "gorm.io/gorm/schema"
)

func main() {
	ctx := context.Background()

	// 1. 创建聊天模型
	cm, err := model.NewChatModel(model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIVersion(os.Getenv("APIVersion")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("gpt-5-mini"),
	)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
		return
	}

	em, err := model.NewEmbModel(model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIVersion(os.Getenv("APIVersion")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("text-embedding-3-large"),
	)
	if err != nil {
		log.Fatalf("创建嵌入模型失败: %v", err)
		return
	}

	gormSql, err := NewMysqlGrom("root:123456@tcp(127.0.0.1:3306)/aggo", logger.Silent)

	client, err := milvusclient.New(context.Background(), &milvusclient.ClientConfig{
		Address: "127.0.0.1:19530",
		DBName:  "",
	})
	if err != nil {
		return
	}

	// 2. 创建向量数据库（使用 Milvus）
	vectorDB, err := vectordb.NewMilvusVectorDB(vectordb.MilvusConfig{
		Client:         client,
		EmbeddingDim:   1024,
		CollectionName: "aggo",
	})
	if err != nil {
		log.Fatalf("创建向量数据库失败: %v", err)
		return
	}

	_storage, err := storage.NewGormStorage(gormSql)
	if err != nil {
		log.Fatalf("创建存储失败: %v", err)
		return
	}
	// 3. 创建知识库管理器
	knowledgeManager, err := knowledge.NewKnowledgeManager(&knowledge.KnowledgeConfig{
		Storage:              _storage,
		VectorDB:             vectorDB,
		Em:                   em,
		DefaultSearchOptions: knowledge.SearchOptions{},
		DefaultLoadOptions:   knowledge.LoadOptions{},
	})
	if err != nil {
		log.Fatalf("创建知识库管理器失败: %v", err)
		return
	}
	log.Println("开始添加数据")

	// 4. 加载示例文档到知识库（模拟一些技术文档）
	docs := []knowledge.Document{
		{
			ID:      utils.GetUUIDNoDash(),
			Content: "Go语言是由Google开发的开源编程语言，以其简洁性和高效性著称。它特别适用于系统编程、网络服务和云计算应用。",
			Metadata: map[string]interface{}{
				"title":  "Go语言介绍",
				"source": "技术文档",
				"type":   "编程语言",
			},
		},
		{
			ID:      utils.GetUUIDNoDash(),
			Content: "微服务架构是一种将单一应用程序开发为一套小服务的方法，每个服务运行在自己的进程中，服务间通过HTTP API进行通信。",
			Metadata: map[string]interface{}{
				"title":  "微服务架构",
				"source": "架构文档",
				"type":   "系统架构",
			},
		},
		{
			ID:      utils.GetUUIDNoDash(),
			Content: "Docker是一个开源的应用容器引擎，让开发者可以打包他们的应用以及依赖包到一个可移植的镜像中，然后发布到任何Linux或Windows机器上。",
			Metadata: map[string]interface{}{
				"title":  "Docker容器技术",
				"source": "技术文档",
				"type":   "容器技术",
			},
		},
	}

	reader := readers.NewInMemoryReader(docs)
	documents, err := reader.ReadDocuments(ctx)
	if err != nil {
		log.Fatalf("读取文档失败: %v", err)
	}

	err = knowledgeManager.LoadDocuments(ctx, documents, knowledge.LoadOptions{
		Recreate:       true,
		Upsert:         false,
		EnableChunking: true, // 禁用分块以简化测试
	})
	if err != nil {
		log.Fatalf("加载文档到知识库失败: %v", err)
	}

	log.Println("知识库初始化完成，已加载示例文档")
	if true {
		return
	}
	// 5. 创建主 Agent，将 KnowledgeAgent 作为工具
	mainAgent, err := agent.NewAgent(ctx, cm,
		agent.WithKnowledgeManager(knowledgeManager),
		agent.WithKnowledgeQueryConfig(&agent.KnowledgeQueryConfig{
			MaxResults:  3,
			Threshold:   0.7,
			AlwaysQuery: false,
		}),
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

	for i, question := range testQuestions {
		log.Printf("\n=== 测试问题 %d ===", i+1)
		log.Printf("用户: %s", question)

		response, err := mainAgent.Generate(ctx, []*schema.Message{
			schema.UserMessage(question),
		})
		if err != nil {
			log.Printf("生成回答失败: %v", err)
			continue
		}

		log.Printf("AI助手: %s", response.Content)
	}
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

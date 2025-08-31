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

	log.Println("=== Storage 层与 VectorDB 层集成示例 ===")

	// 1. 创建嵌入模型
	em, err := model.NewEmbModel()
	if err != nil {
		log.Fatalf("创建嵌入模型失败: %v", err)
	}

	// 2. 创建 Storage 层（SQLite）
	store, err := storage.NewSQLiteStorage("integrated_knowledge.db")
	if err != nil {
		log.Fatalf("创建存储层失败: %v", err)
	}
	defer store.Close()
	log.Println("✓ Storage 层初始化完成")

	// 3. 创建 VectorDB 层（Milvus）
	vectorDB, err := vectordb.NewMilvusVectorDB(vectordb.MilvusConfig{
		Address:        "127.0.0.1:19530",
		CollectionName: "knowledge_vectors",
		EmbeddingDim:   1024, // 根据嵌入模型维度设置
	})
	if err != nil {
		log.Fatalf("创建向量数据库失败: %v", err)
	}
	defer vectorDB.Close()
	log.Println("✓ VectorDB 层初始化完成")

	// 4. 创建 KnowledgeManager 协调两个存储层
	manager, err := knowledge.NewKnowledgeManager(&knowledge.KnowledgeConfig{
		Storage:  store,    // Storage 层：处理文档基本信息
		VectorDB: vectorDB, // VectorDB 层：处理向量数据
		Em:       em,       // 嵌入模型
		DefaultSearchOptions: knowledge.SearchOptions{
			Limit:     5,
			Threshold: 0.7,
		},
		DefaultLoadOptions: knowledge.LoadOptions{
			EnableChunking: true,
			ChunkSize:      1000,
			ChunkOverlap:   200,
		},
	})
	if err != nil {
		log.Fatalf("创建知识库管理器失败: %v", err)
	}
	log.Println("✓ KnowledgeManager 初始化完成")

	// 5. 测试数据
	testDocuments := []knowledge.Document{
		{
			ID:      "tech_go",
			Content: "Go语言是由Google开发的开源编程语言，以其简洁的语法、高效的并发处理和快速的编译速度而著称。Go特别适用于构建网络服务、分布式系统和云原生应用。",
			Metadata: map[string]interface{}{
				"title":    "Go语言介绍",
				"category": "编程语言",
				"author":   "Google",
				"tags":     []string{"golang", "编程", "后端"},
			},
		},
		{
			ID:      "tech_docker",
			Content: "Docker是一个开源的应用容器引擎，让开发者可以打包他们的应用以及依赖包到一个可移植的镜像中。Docker使用Linux容器技术，提供轻量级的虚拟化解决方案。",
			Metadata: map[string]interface{}{
				"title":    "Docker容器技术",
				"category": "容器技术",
				"tags":     []string{"docker", "容器", "虚拟化"},
			},
		},
		{
			ID:      "tech_k8s",
			Content: "Kubernetes是一个开源的容器编排平台，用于自动化应用程序的部署、扩展和管理。它提供了服务发现、负载均衡、存储编排等企业级功能。",
			Metadata: map[string]interface{}{
				"title":    "Kubernetes容器编排",
				"category": "容器编排",
				"tags":     []string{"kubernetes", "k8s", "容器编排"},
			},
		},
	}

	// 6. 加载文档（KnowledgeManager 会自动协调 Storage 和 VectorDB）
	log.Println("6. 加载文档到知识库...")
	err = manager.LoadDocuments(ctx, testDocuments, knowledge.LoadOptions{
		EnableChunking: false, // 文档较短，不需要分块
		Upsert:         true,
	})
	if err != nil {
		log.Fatalf("加载文档失败: %v", err)
	}
	log.Printf("✓ 成功加载 %d 个文档\n", len(testDocuments))

	// 7. 演示数据分离：直接访问 Storage 层
	log.Println("7. 直接访问 Storage 层（仅文档基本信息）...")
	storageDoc, err := store.GetDocument(ctx, "tech_go")
	if err != nil {
		log.Fatalf("从 Storage 获取文档失败: %v", err)
	}
	log.Printf("✓ Storage 层文档: ID=%s, Content=%s..., Vector=%v\n",
		storageDoc.ID, storageDoc.Content[:30], storageDoc.Vector == nil)

	// 8. 演示数据分离：直接访问 VectorDB 层
	log.Println("8. 直接访问 VectorDB 层（向量搜索）...")
	vectorResults, err := vectorDB.SearchByText(ctx, "Go语言编程", 2, nil)
	if err != nil {
		log.Fatalf("VectorDB 搜索失败: %v", err)
	}
	log.Printf("✓ VectorDB 搜索结果: %d 个文档\n", len(vectorResults))
	for i, result := range vectorResults {
		log.Printf("  %d. ID=%s, Score=%.3f, Content=%s...\n",
			i+1, result.Document.ID, result.Score, result.Document.Content[:40])
	}

	// 9. 演示 KnowledgeManager 的统一接口
	log.Println("9. 通过 KnowledgeManager 进行语义搜索...")
	searchResults, err := manager.Search(ctx, "容器技术和编排", knowledge.SearchOptions{
		Limit:     3,
		Threshold: 0.6,
	})
	if err != nil {
		log.Fatalf("知识库搜索失败: %v", err)
	}
	log.Printf("✓ 语义搜索结果: %d 个文档\n", len(searchResults))
	for i, result := range searchResults {
		log.Printf("  %d. ID=%s, Score=%.3f\n", i+1, result.Document.ID, result.Score)
		log.Printf("     标题: %v\n", result.Document.Metadata["title"])
		log.Printf("     内容: %s...\n", result.Document.Content[:60])
	}

	// 10. 演示 Storage 层的文本搜索（非语义）
	log.Println("10. Storage 层文本搜索（非语义）...")
	textSearchResults, err := store.SearchDocuments(ctx, "Docker", 3)
	if err != nil {
		log.Fatalf("Storage 文本搜索失败: %v", err)
	}
	log.Printf("✓ 文本搜索结果: %d 个文档\n", len(textSearchResults))
	for i, doc := range textSearchResults {
		log.Printf("  %d. ID=%s, 标题=%v\n", i+1, doc.ID, doc.Metadata["title"])
	}

	log.Println("\n=== 架构验证总结 ===")
	log.Println("✓ Storage 层专门处理文档基本信息（ID、Content、Metadata、时间戳）")
	log.Println("✓ VectorDB 层专门处理向量数据和语义搜索")
	log.Println("✓ KnowledgeManager 协调两个存储层，提供统一接口")
	log.Println("✓ 数据职责分离明确，各层可独立扩展和优化")
}

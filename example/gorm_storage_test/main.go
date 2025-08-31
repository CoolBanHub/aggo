package main

import (
	"context"
	"log"
	"time"

	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/knowledge/storage"
)

func main() {
	ctx := context.Background()

	// 测试 SQLite 存储
	log.Println("=== 测试 SQLite 存储 ===")
	testSQLiteStorage(ctx)

	// 测试 MySQL 存储（需要 MySQL 服务运行）
	// log.Println("=== 测试 MySQL 存储 ===")
	// testMySQLStorage(ctx)

	// 测试 PostgreSQL 存储（需要 PostgreSQL 服务运行）
	// log.Println("=== 测试 PostgreSQL 存储 ===")
	// testPostgreSQLStorage(ctx)
}

func testSQLiteStorage(ctx context.Context) {
	// 创建 SQLite 配置
	config := storage.NewSQLiteConfig("test_knowledge.db")
	config.LogLevel = 4 // 开启详细日志

	// 创建存储实例
	store, err := storage.NewGormStorage(config)
	if err != nil {
		log.Fatalf("Failed to create SQLite storage: %v", err)
	}
	defer store.Close()

	// 运行通用测试
	testKnowledgeStorage(ctx, store, "SQLite")
}

func testMySQLStorage(ctx context.Context) {
	// 创建 MySQL 配置
	config := storage.NewMySQLConfig("localhost", 3306, "test_knowledge", "root", "password")
	config.LogLevel = 4 // 开启详细日志

	// 创建存储实例
	store, err := storage.NewGormStorage(config)
	if err != nil {
		log.Fatalf("Failed to create MySQL storage: %v", err)
	}
	defer store.Close()

	// 运行通用测试
	testKnowledgeStorage(ctx, store, "MySQL")
}

func testPostgreSQLStorage(ctx context.Context) {
	// 创建 PostgreSQL 配置
	config := storage.NewPostgreSQLConfig("localhost", 5432, "test_knowledge", "postgres", "password")
	config.LogLevel = 4 // 开启详细日志

	// 创建存储实例
	store, err := storage.NewGormStorage(config)
	if err != nil {
		log.Fatalf("Failed to create PostgreSQL storage: %v", err)
	}
	defer store.Close()

	// 运行通用测试
	testKnowledgeStorage(ctx, store, "PostgreSQL")
}

func testKnowledgeStorage(ctx context.Context, store *storage.GormStorage, dbType string) {
	log.Printf("开始测试 %s 存储功能...\n", dbType)

	// 1. 测试保存文档
	log.Println("1. 测试保存文档...")
	doc1 := &knowledge.Document{
		ID:      "doc1",
		Content: "Go语言是由Google开发的开源编程语言，以其简洁性和高效性著称。",
		Metadata: map[string]interface{}{
			"title":   "Go语言介绍",
			"author":  "Google",
			"topic":   "编程语言",
			"version": 1.0,
		},
		// 注意：向量数据不在 Storage 层处理，由 VectorDB 负责
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.SaveDocument(ctx, doc1); err != nil {
		log.Fatalf("Failed to save document: %v", err)
	}
	log.Printf("✓ 文档 %s 保存成功\n", doc1.ID)

	// 2. 测试获取文档
	log.Println("2. 测试获取文档...")
	retrievedDoc, err := store.GetDocument(ctx, "doc1")
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}
	log.Printf("✓ 获取文档成功: %s\n", retrievedDoc.Content[:20]+"...")
	log.Printf("  元数据: %+v\n", retrievedDoc.Metadata)
	log.Printf("  向量: %v (由 VectorDB 管理)\n", retrievedDoc.Vector)

	// 3. 测试批量保存文档
	log.Println("3. 测试批量保存文档...")
	docs := []*knowledge.Document{
		{
			ID:      "doc2",
			Content: "微服务架构是一种将单一应用程序开发为一套小服务的方法。",
			Metadata: map[string]interface{}{
				"title": "微服务架构",
				"topic": "系统架构",
			},
		},
		{
			ID:      "doc3",
			Content: "Docker是一个开源的应用容器引擎。",
			Metadata: map[string]interface{}{
				"title": "Docker介绍",
				"topic": "容器技术",
			},
		},
		{
			ID:      "doc4",
			Content: "Kubernetes是一个开源的容器编排平台。",
			Metadata: map[string]interface{}{
				"title": "Kubernetes介绍",
				"topic": "容器编排",
			},
		},
	}

	if err := store.BatchSaveDocuments(ctx, docs, 2); err != nil {
		log.Fatalf("Failed to batch save documents: %v", err)
	}
	log.Printf("✓ 批量保存 %d 个文档成功\n", len(docs))

	// 4. 测试文档统计
	log.Println("4. 测试文档统计...")
	count, err := store.Count(ctx)
	if err != nil {
		log.Fatalf("Failed to count documents: %v", err)
	}
	log.Printf("✓ 文档总数: %d\n", count)

	// 5. 测试列出文档
	log.Println("5. 测试列出文档...")
	allDocs, err := store.ListDocuments(ctx, 10, 0)
	if err != nil {
		log.Fatalf("Failed to list documents: %v", err)
	}
	log.Printf("✓ 列出文档成功，共 %d 个文档:\n", len(allDocs))
	for i, doc := range allDocs {
		log.Printf("  %d. %s: %s\n", i+1, doc.ID, doc.Content[:30]+"...")
	}

	// 6. 测试搜索文档
	log.Println("6. 测试搜索文档...")
	searchResults, err := store.SearchDocuments(ctx, "Go语言", 5)
	if err != nil {
		log.Fatalf("Failed to search documents: %v", err)
	}
	log.Printf("✓ 搜索 'Go语言' 结果，共 %d 个文档:\n", len(searchResults))
	for i, doc := range searchResults {
		log.Printf("  %d. %s: %s\n", i+1, doc.ID, doc.Content[:50]+"...")
	}

	// 7. 测试更新文档
	log.Println("7. 测试更新文档...")
	doc1.Content = "Go语言是由Google开发的开源编程语言，以其简洁性、高效性和并发性著称。[已更新]"
	doc1.Metadata["version"] = 2.0
	// 注意：向量更新需要在 VectorDB 层处理

	if err := store.UpdateDocument(ctx, doc1); err != nil {
		log.Fatalf("Failed to update document: %v", err)
	}
	log.Printf("✓ 文档 %s 更新成功\n", doc1.ID)

	// 验证更新
	updatedDoc, err := store.GetDocument(ctx, "doc1")
	if err != nil {
		log.Fatalf("Failed to get updated document: %v", err)
	}
	log.Printf("  更新后内容: %s\n", updatedDoc.Content[:50]+"...")
	log.Printf("  更新后元数据: %+v\n", updatedDoc.Metadata)

	// 8. 测试删除文档
	log.Println("8. 测试删除文档...")
	if err := store.DeleteDocument(ctx, "doc4"); err != nil {
		log.Fatalf("Failed to delete document: %v", err)
	}
	log.Printf("✓ 文档 doc4 删除成功\n")

	// 验证删除
	_, err = store.GetDocument(ctx, "doc4")
	if err == nil {
		log.Fatal("Document doc4 should have been deleted")
	}
	log.Printf("✓ 确认文档 doc4 已删除\n")

	// 最终统计
	finalCount, err := store.Count(ctx)
	if err != nil {
		log.Fatalf("Failed to get final count: %v", err)
	}
	log.Printf("✓ 最终文档总数: %d\n", finalCount)

	log.Printf("=== %s 存储测试完成 ===\n\n", dbType)
}

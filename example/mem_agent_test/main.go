package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/storage"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino/schema"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	schemaGrom "gorm.io/gorm/schema"
)

func main() {
	ctx := context.Background()
	cm, err := model.NewChatModel(model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("gpt-5-mini"),
	)
	if err != nil {
		log.Fatalf("new chat model fail,err:%s", err)
		return
	}
	gormSql, err := NewMysqlGrom("root:123456@tcp(127.0.0.1:3306)/aggo", logger.Silent)
	if err != nil {
		log.Fatalf("创建数据库连接失败: %v", err)
		return
	}
	s, err := storage.NewGormStorage(gormSql)
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
	sessionID := utils.GetUUIDNoDash()
	bot, err := agent.NewAgent(ctx, cm,
		agent.WithMemoryManager(memoryManager),
		agent.WithSystemPrompt("你是一名28岁的厦门女孩,名叫mary,是一名短视频剪辑师，主要剪辑电影类。喜欢养狗，家里有一只金毛，它叫小黄。喜欢宅在家里刷剧。不喜欢吃水果。性格内向高冷，别人不问的时候，一般都不回答自己的信息"),
	)
	if err != nil {
		log.Fatalf("new agent fail,err:%s", err)
		return
	}

	conversations := []string{
		"你好，我是Alice",
		"我是一名软件工程师，专门做后端开发",
		"我住在北京，今年28岁",
		"你有什么爱好吗?",
		//"我喜欢读书和摄影，特别是科幻小说",
		//"我最近在学习Go语言和云原生技术",
		//"我的工作主要涉及微服务架构设计",
		//"周末我通常会去公园拍照或者在家看书",
		//"你能给我推荐一些适合我的技术书籍吗？",
		//"你还记得我之前说过我的职业是什么吗？",
		//"基于你对我的了解，你觉得我适合学习什么新技术？",
		//"我们年龄相差多少岁呢",
		//"你喜欢吃什么水果吗？我喜欢吃苹果",
		//"你知道我的住哪里吗",
	}

	for _, conversation := range conversations {
		log.Printf("User: %s", conversation)
		out, err := bot.Generate(ctx, []*schema.Message{
			schema.UserMessage(conversation),
		}, agent.WithChatSessionID(sessionID), agent.WithChatUserID(sessionID))
		if err != nil {
			log.Fatalf("generate fail,err:%s", err)
			return
		}
		log.Printf("AI:%s", out.Content)
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

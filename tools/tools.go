// Package tools 提供 AI Agent 工具的统一入口。
//
// 建议直接导入子 package 使用：
//
//	import "github.com/CoolBanHub/aggo/tools/database/mysql"
//	import "github.com/CoolBanHub/aggo/tools/database/postgres"
//	import "github.com/CoolBanHub/aggo/tools/knowledge"
//	import "github.com/CoolBanHub/aggo/tools/shell"
//	import "github.com/CoolBanHub/aggo/tools/cron"
package tools

import (
	cronPkg "github.com/CoolBanHub/aggo/cron"
	cronTool "github.com/CoolBanHub/aggo/tools/cron"
	"github.com/CoolBanHub/aggo/tools/database/mysql"
	"github.com/CoolBanHub/aggo/tools/database/postgres"
	"github.com/CoolBanHub/aggo/tools/knowledge"
	"github.com/CoolBanHub/aggo/tools/shell"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"gorm.io/gorm"
)

// ============================================================
// Database 工具
// ============================================================

// GetMySQLTools 获取 MySQL 工具列表
func GetMySQLTools(db *gorm.DB) []tool.BaseTool {
	return mysql.GetTools(db)
}

// GetPostgresTools 获取 PostgreSQL 工具列表
func GetPostgresTools(db *gorm.DB) []tool.BaseTool {
	return postgres.GetTools(db)
}

// ============================================================
// Knowledge 工具
// ============================================================

// GetKnowledgeTools 获取知识库管理工具
func GetKnowledgeTools(indexer indexer.Indexer, retriever retriever.Retriever, retrieverOptions *retriever.Options) []tool.BaseTool {
	return knowledge.GetTools(indexer, retriever, retrieverOptions)
}

// GetKnowledgeReasoningTools 获取知识推理工具
func GetKnowledgeReasoningTools(r retriever.Retriever, retrieverOptions []retriever.Option) []tool.BaseTool {
	return knowledge.GetReasoningTools(r, retrieverOptions)
}

// ============================================================
// Shell 工具
// ============================================================

// GetShellTools 获取全部 Shell 工具
func GetShellTools() []tool.BaseTool {
	return shell.GetTools()
}

// GetSysInfoTools 获取系统信息工具
func GetSysInfoTools() []tool.BaseTool {
	return shell.GetSysInfoTools()
}

// GetExecuteTools 获取命令执行工具
func GetExecuteTools() []tool.BaseTool {
	return shell.GetExecuteTools()
}

// ============================================================
// Cron 定时任务工具
// ============================================================

// GetCronTools 获取定时任务工具
func GetCronTools(service *cronPkg.CronService, opts ...cronTool.CronOption) []tool.BaseTool {
	return cronTool.GetTools(service, opts...)
}

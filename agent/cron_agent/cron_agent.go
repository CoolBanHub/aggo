// Package cron_agent 提供开箱即用的定时任务 Agent。
//
// CronAgent 是一个预配置的 Agent，内置了 cron 工具，可以直接用于管理定时任务。
// 它封装了 CronService 的生命周期管理（Start/Stop），提供简单的创建和使用接口。
//
// 基本用法：
//
//	// 使用文件存储
//	cronAgent, err := cron_agent.New(ctx, cm,
//	    cron_agent.WithFileStore("/path/to/cron_jobs.json"),
//	)
//
//	// 使用数据库存储
//	cronAgent, err := cron_agent.New(ctx, cm,
//	    cron_agent.WithGormStore(db),
//	)
//
//	// 启动调度
//	cronAgent.Start()
//	defer cronAgent.Stop()
//
//	// 像普通 Agent 一样使用
//	resp, err := cronAgent.Generate(ctx, messages)
package cron_agent

import (
	"context"
	"fmt"

	"github.com/CoolBanHub/aggo/agent"
	cronPkg "github.com/CoolBanHub/aggo/cron"
	cronTool "github.com/CoolBanHub/aggo/tools/cron"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"gorm.io/gorm"
)

// CronAgent 定时任务 Agent
type CronAgent struct {
	*agent.Agent
	service *cronPkg.CronService
}

// Option 配置选项
type Option func(*config)

type config struct {
	store          cronPkg.Store
	onJobTriggered func(job *cronPkg.CronJob)
	agentOpts      []agent.Option
	extraTools     []tool.BaseTool
}

// WithFileStore 使用文件存储
func WithFileStore(path string) Option {
	return func(c *config) {
		c.store = cronPkg.NewFileStore(path)
	}
}

// WithGormStore 使用 GORM 数据库存储（支持 MySQL、PostgreSQL、SQLite）
func WithGormStore(db *gorm.DB) Option {
	return func(c *config) {
		store, err := cronPkg.NewGormStore(db)
		if err != nil {
			panic(fmt.Sprintf("failed to create gorm store: %v", err))
		}
		c.store = store
	}
}

// WithStore 使用自定义存储实现
func WithStore(store cronPkg.Store) Option {
	return func(c *config) {
		c.store = store
	}
}

// WithOnJobTriggered 设置任务触发回调
func WithOnJobTriggered(fn func(job *cronPkg.CronJob)) Option {
	return func(c *config) {
		c.onJobTriggered = fn
	}
}

// WithAgentOptions 设置 Agent 的附加选项（如 WithName、WithMemoryManager 等）
func WithAgentOptions(opts ...agent.Option) Option {
	return func(c *config) {
		c.agentOpts = append(c.agentOpts, opts...)
	}
}

// WithExtraTools 添加额外的工具
func WithExtraTools(tools ...tool.BaseTool) Option {
	return func(c *config) {
		c.extraTools = append(c.extraTools, tools...)
	}
}

// New 创建定时任务 Agent
//
// 必须通过 WithFileStore、WithGormStore 或 WithStore 指定存储方式。
// 创建后需调用 Start() 启动调度循环，Stop() 停止。
func New(ctx context.Context, cm model.ToolCallingChatModel, opts ...Option) (*CronAgent, error) {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.store == nil {
		return nil, fmt.Errorf("store is required, use WithFileStore, WithGormStore, or WithStore")
	}

	// 创建 CronService
	service := cronPkg.NewCronService(cfg.store, nil)

	// 构建 cron 工具
	var cronOpts []cronTool.CronOption
	if cfg.onJobTriggered != nil {
		cronOpts = append(cronOpts, cronTool.WithOnJobTriggered(cfg.onJobTriggered))
	}
	tools := cronTool.GetTools(service, cronOpts...)

	// 合并额外工具
	if len(cfg.extraTools) > 0 {
		tools = append(tools, cfg.extraTools...)
	}

	// 构建 agent 选项
	agentOpts := []agent.Option{
		agent.WithName("定时任务助手"),
		agent.WithDescription("专业的定时任务管理助手，可以添加、查看、删除、启用和禁用定时任务"),
		agent.WithSystemPrompt(
			"你是一个专业的定时任务管理助手。你可以帮助用户：\n" +
				"1. 添加定时任务：支持一次性定时（at_seconds）、周期定时（every_seconds）和 Cron 表达式（cron_expr）\n" +
				"2. 查看所有定时任务\n" +
				"3. 删除定时任务\n" +
				"4. 启用或禁用定时任务\n\n" +
				"当用户要求设置提醒或定时任务时，请使用 cron 工具。\n" +
				"对于简单的提醒（如 '10分钟后提醒我'），使用 at_seconds。\n" +
				"对于周期性任务（如 '每2小时提醒我'），使用 every_seconds。\n" +
				"对于复杂调度（如 '每天早上9点'），使用 cron_expr。",
		),
		agent.WithTools(tools),
		agent.WithMaxStep(5),
	}

	// 用户的选项优先级更高，放在后面覆盖默认值
	agentOpts = append(agentOpts, cfg.agentOpts...)

	// 创建 Agent
	ag, err := agent.NewAgent(ctx, cm, agentOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create cron agent: %w", err)
	}

	return &CronAgent{
		Agent:   ag,
		service: service,
	}, nil
}

// Start 启动定时调度服务
func (ca *CronAgent) Start() error {
	return ca.service.Start()
}

// Stop 停止定时调度服务
func (ca *CronAgent) Stop() {
	ca.service.Stop()
}

// Service 返回内部的 CronService，用于直接操作
func (ca *CronAgent) Service() *cronPkg.CronService {
	return ca.service
}

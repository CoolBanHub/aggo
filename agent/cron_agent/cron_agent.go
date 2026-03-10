// Package cron_agent 提供开箱即用的定时任务 Agent。
//
// CronAgent 是一个预配置的 Agent，内置了 cron 工具，可以直接用于管理定时任务。
// 它封装了 CronService 的生命周期管理（Start/Stop），提供简单的创建和使用接口。
//
// 任务触发时，默认会将 job.Payload.Message 作为用户消息回送给 Agent 处理，
// 使得 Agent 可以在任务触发时执行更多操作（如创建新的定时任务）。
// 可通过 WithOnJobTriggered 覆盖此默认行为。
//
// 安全机制：
//   - 单用户任务数上限（默认 10），防止单用户占用过多资源
//   - 系统提示词禁止创建嵌套周期性任务
//
// 基本用法：
//
//	cronAgent, err := cron_agent.New(ctx, cm,
//	    cron_agent.WithFileStore("/path/to/cron_jobs.json"),
//	)
//	cronAgent.Start()
//	defer cronAgent.Stop()
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
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

const (
	defaultMaxJobsPerUser = 10 // 默认单用户最大任务数
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
	onJobProcessed func(job *cronPkg.CronJob, response string, err error)
	agentOpts      []agent.Option
	extraTools     []tool.BaseTool
	maxJobs        int // 任务总数上限，0 表示不限制（默认不限制）
	maxJobsPerUser int // 单用户任务数上限
	locker         cronPkg.Locker
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

// WithOnJobTriggered 设置自定义的任务触发回调。
// 设置后将覆盖默认的自动回送 Agent 处理行为。
func WithOnJobTriggered(fn func(job *cronPkg.CronJob)) Option {
	return func(c *config) {
		c.onJobTriggered = fn
	}
}

// WithOnJobProcessed 设置任务被 Agent 自动处理后的回调。
// 仅在使用默认自动处理（未设置 WithOnJobTriggered）时生效。
// 可用于记录日志、发送通知等。
func WithOnJobProcessed(fn func(job *cronPkg.CronJob, response string, err error)) Option {
	return func(c *config) {
		c.onJobProcessed = fn
	}
}

// WithLocker 设置分布式锁
func WithLocker(locker cronPkg.Locker) Option {
	return func(c *config) {
		c.locker = locker
	}
}

// WithMaxJobs 设置最大任务总数限制。默认不限制。
func WithMaxJobs(max int) Option {
	return func(c *config) {
		c.maxJobs = max
	}
}

// WithMaxJobsPerUser 设置单用户最大任务数量限制。默认 10。
func WithMaxJobsPerUser(max int) Option {
	return func(c *config) {
		c.maxJobsPerUser = max
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
//
// 默认行为：任务触发时，将 message 回送给 Agent 处理（支持嵌套任务创建）。
// 可通过 WithOnJobTriggered 覆盖为自定义处理逻辑。
func New(ctx context.Context, cm model.ToolCallingChatModel, opts ...Option) (*CronAgent, error) {
	cfg := &config{
		maxJobsPerUser: defaultMaxJobsPerUser,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.store == nil {
		return nil, fmt.Errorf("store is required, use WithFileStore, WithGormStore, or WithStore")
	}

	// 创建 CronService
	service := cronPkg.NewCronService(cfg.store, nil)

	// 设置任务数量上限
	if cfg.maxJobs > 0 {
		service.SetMaxJobs(cfg.maxJobs)
	}
	if cfg.maxJobsPerUser > 0 {
		service.SetMaxJobsPerUser(cfg.maxJobsPerUser)
	}

	// 设置分布式锁
	if cfg.locker != nil {
		service.SetLocker(cfg.locker)
	}

	// 构建 cron 工具（先不设置回调，在 agent 创建后再设置）
	tools := cronTool.GetTools(service)

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
				"对于复杂调度（如 '每天早上9点'），使用 cron_expr。\n\n" +
				"【重要安全规则】\n" +
				"禁止创建会自动生成新的周期性定时任务的定时任务，这会导致任务无限增长。\n" +
				"例如：禁止 '每60秒创建一个每10秒执行的任务' 这种嵌套周期性任务。\n" +
				"如果用户请求此类操作，请拒绝并解释风险。\n" +
				"允许的模式：周期性任务创建一次性任务（如 '每60秒创建一个10秒后的提醒'）。",
		),
		agent.WithTools(tools),
	}

	// 用户的选项优先级更高，放在后面覆盖默认值
	agentOpts = append(agentOpts, cfg.agentOpts...)

	// 创建 Agent
	ag, err := agent.NewAgent(ctx, cm, agentOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create cron agent: %w", err)
	}

	ca := &CronAgent{
		Agent:   ag,
		service: service,
	}

	// 设置任务触发回调
	if cfg.onJobTriggered != nil {
		// 用户自定义回调
		service.SetOnJob(func(job *cronPkg.CronJob) (string, error) {
			cfg.onJobTriggered(job)
			return "ok", nil
		})
	} else {
		// 默认行为：将 message 回送给 Agent 处理
		onProcessed := cfg.onJobProcessed
		service.SetOnJob(func(job *cronPkg.CronJob) (string, error) {
			resp, err := cm.Generate(context.Background(), []*schema.Message{
				schema.SystemMessage("你是一个提醒助手。请将以下定时任务消息转换为简洁、友好的提醒通知。要求：直接输出一句话，不要解释、不要提供多个版本、不要询问用户偏好。格式：🔔 [提醒] {内容}"),
				schema.UserMessage(job.Payload.Message),
			})

			var response string
			if resp != nil {
				response = resp.Content
			}

			if onProcessed != nil {
				onProcessed(job, response, err)
			}

			return response, err
		})
	}

	return ca, nil
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

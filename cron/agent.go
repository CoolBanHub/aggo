package cron

import (
	"context"
	"fmt"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultMaxJobsPerUser = 10
)

// CronAgentOption 配置选项
type CronAgentOption func(*cronConfig)

type cronConfig struct {
	store          Store
	onJobTriggered func(job *CronJob)
	onJobProcessed func(job *CronJob, response string, err error)
	middlewares    []adk.ChatModelAgentMiddleware
	extraTools     []tool.BaseTool
	maxJobs        int
	maxJobsPerUser int
	locker         Locker
	memoryProvider  memory.MemoryProvider
	name           string
	systemPrompt   string
}

// WithFileStore 使用文件存储
func WithFileStore(path string) CronAgentOption {
	return func(c *cronConfig) {
		c.store = NewFileStore(path)
	}
}

// WithCronStore 使用自定义存储实现
func WithCronStore(store Store) CronAgentOption {
	return func(c *cronConfig) {
		c.store = store
	}
}

// WithOnJobTriggered 设置自定义的任务触发回调
func WithOnJobTriggered(fn func(job *CronJob)) CronAgentOption {
	return func(c *cronConfig) {
		c.onJobTriggered = fn
	}
}

// WithOnJobProcessed 设置任务被 Agent 自动处理后的回调
func WithOnJobProcessed(fn func(job *CronJob, response string, err error)) CronAgentOption {
	return func(c *cronConfig) {
		c.onJobProcessed = fn
	}
}

// WithCronLocker 设置分布式锁
func WithCronLocker(locker Locker) CronAgentOption {
	return func(c *cronConfig) {
		c.locker = locker
	}
}

// WithMaxJobs 设置最大任务总数限制
func WithMaxJobs(max int) CronAgentOption {
	return func(c *cronConfig) {
		c.maxJobs = max
	}
}

// WithMaxJobsPerUser 设置单用户最大任务数量限制
func WithMaxJobsPerUser(max int) CronAgentOption {
	return func(c *cronConfig) {
		c.maxJobsPerUser = max
	}
}

// WithExtraTools 添加额外的工具
func WithExtraTools(tools ...tool.BaseTool) CronAgentOption {
	return func(c *cronConfig) {
		c.extraTools = append(c.extraTools, tools...)
	}
}

// WithCronMiddlewares 添加 Middleware
func WithCronMiddlewares(mw ...adk.ChatModelAgentMiddleware) CronAgentOption {
	return func(c *cronConfig) {
		c.middlewares = append(c.middlewares, mw...)
	}
}

// WithCronMemory 设置 MemoryProvider
func WithCronMemory(provider memory.MemoryProvider) CronAgentOption {
	return func(c *cronConfig) {
		c.memoryProvider = provider
	}
}

// WithCronName 设置 Agent 名称
func WithCronName(name string) CronAgentOption {
	return func(c *cronConfig) {
		c.name = name
	}
}

// WithCronSystemPrompt 设置系统提示词
func WithCronSystemPrompt(prompt string) CronAgentOption {
	return func(c *cronConfig) {
		c.systemPrompt = prompt
	}
}

// CronAgentResult 定时任务 Agent 创建结果
type CronAgentResult struct {
	Agent   adk.Agent
	Service *CronService
}

type cronServiceProvider interface {
	CronService() *CronService
}

// NewCronAgent 创建定时任务 Agent
// tools 参数由调用方通过 cronTool.GetTools(service) 提供，避免循环导入
func NewCronAgent(ctx context.Context, cm model.ToolCallingChatModel, cronTools []tool.BaseTool, opts ...CronAgentOption) (*CronAgentResult, error) {
	cfg := &cronConfig{
		maxJobsPerUser: defaultMaxJobsPerUser,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	service := extractCronService(cronTools)
	if service == nil {
		if cfg.store == nil {
			return nil, fmt.Errorf("store is required when cron tools are not bound to a CronService")
		}
		service = NewCronService(cfg.store, nil)
	}

	if cfg.maxJobs > 0 {
		service.SetMaxJobs(cfg.maxJobs)
	}
	if cfg.maxJobsPerUser > 0 {
		service.SetMaxJobsPerUser(cfg.maxJobsPerUser)
	}
	if cfg.locker != nil {
		service.SetLocker(cfg.locker)
	}

	// 合并工具
	allTools := make([]tool.BaseTool, 0, len(cronTools)+len(cfg.extraTools))
	allTools = append(allTools, cronTools...)
	allTools = append(allTools, cfg.extraTools...)

	// 构建 Middleware 链
	handlers := make([]adk.ChatModelAgentMiddleware, 0, len(cfg.middlewares)+1)
	if cfg.memoryProvider != nil {
		handlers = append(handlers, memory.NewMemoryMiddleware(cfg.memoryProvider))
	}
	handlers = append(handlers, cfg.middlewares...)

	name := cfg.name
	if name == "" {
		name = "定时任务助手"
	}
	description := "专业的定时任务管理助手"
	systemPrompt := cfg.systemPrompt
	if systemPrompt == "" {
		systemPrompt = "你是一个专业的定时任务管理助手。你可以帮助用户：\n" +
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
			"如果用户请求此类操作，请拒绝并解释风险。"
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        name,
		Description: description,
		Instruction: systemPrompt,
		Model:       cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: allTools,
			},
		},
		Handlers: handlers,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cron agent: %w", err)
	}

	// 设置任务触发回调
	if cfg.onJobTriggered != nil {
		service.SetOnJob(func(job *CronJob) (string, error) {
			cfg.onJobTriggered(job)
			return "ok", nil
		})
	} else {
		onProcessed := cfg.onJobProcessed
		service.SetOnJob(func(job *CronJob) (string, error) {
			resp, err := cm.Generate(context.Background(), []*schema.Message{
				schema.SystemMessage("你是一个提醒助手。请将以下定时任务消息转换为简洁、友好的提醒通知。直接输出一句话。"),
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

	return &CronAgentResult{
		Agent:   agent,
		Service: service,
	}, nil
}

func extractCronService(cronTools []tool.BaseTool) *CronService {
	for _, cronTool := range cronTools {
		provider, ok := cronTool.(cronServiceProvider)
		if ok && provider.CronService() != nil {
			return provider.CronService()
		}
	}
	return nil
}

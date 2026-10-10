package cron

import (
	"context"
	"fmt"
	"time"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
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

// JobLifecycle 描述任务触发与处理完成两个阶段的观测回调。
// 两个回调都只用于观测（日志、埋点、通知），不影响模型处理是否执行；
// 如需禁用模型处理，请显式使用 WithDisableJobProcessing。
type JobLifecycle struct {
	// OnTriggered 在任务触发后、模型处理前调用。
	OnTriggered func(ctx context.Context, job *CronJob)
	// OnProcessed 在模型处理结束后调用（无论成功或失败）。
	OnProcessed func(ctx context.Context, job *CronJob, response string, err error)
}

// CronAgentOption 配置选项
type CronAgentOption func(*cronConfig)

type cronConfig struct {
	store    Store
	storeErr error
	// onJobTriggered/onJobProcessed 是观测回调，与模型处理互不排斥
	onJobTriggered func(ctx context.Context, job *CronJob)
	onJobProcessed func(ctx context.Context, job *CronJob, response string, err error)
	middlewares    []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]
	extraTools     []tool.BaseTool
	maxJobs        int
	maxJobsPerUser int
	locker         Locker
	memoryProvider memory.MemoryProvider
	name           string
	systemPrompt   string
	jobTimeout     time.Duration
	jobCtxBuilder  JobContextBuilder
	// disableJobProcessing 显式关闭任务触发后的模型处理
	disableJobProcessing bool
}

// WithFileStore 使用文件存储
func WithFileStore(path string) CronAgentOption {
	return func(c *cronConfig) {
		store, err := NewFileStoreWithError(path)
		c.store = store
		c.storeErr = err
	}
}

// WithCronStore 使用自定义存储实现
func WithCronStore(store Store) CronAgentOption {
	return func(c *cronConfig) {
		c.store = store
	}
}

// WithOnJobTriggered 设置任务触发回调（观测用，不拦截模型处理）。
// 回调在任务触发后、模型调用前执行。
func WithOnJobTriggered(fn func(job *CronJob)) CronAgentOption {
	if fn == nil {
		return func(c *cronConfig) {}
	}
	return WithOnJobTriggeredContext(func(ctx context.Context, job *CronJob) {
		fn(job)
	})
}

// WithOnJobTriggeredContext 设置带任务上下文的触发回调。
// ctx 由 JobContextBuilder 构建，至少携带任务 OwnerID 与 JobID。
func WithOnJobTriggeredContext(fn func(ctx context.Context, job *CronJob)) CronAgentOption {
	return func(c *cronConfig) {
		c.onJobTriggered = fn
	}
}

// WithOnJobProcessed 设置任务被模型处理完成后的回调（无论成功或失败）。
// 模型处理始终执行；如需关闭，请显式使用 WithDisableJobProcessing。
func WithOnJobProcessed(fn func(job *CronJob, response string, err error)) CronAgentOption {
	if fn == nil {
		return func(c *cronConfig) {}
	}
	return WithOnJobProcessedContext(func(ctx context.Context, job *CronJob, response string, err error) {
		fn(job, response, err)
	})
}

// WithOnJobProcessedContext 设置带任务上下文的处理完成回调
func WithOnJobProcessedContext(fn func(ctx context.Context, job *CronJob, response string, err error)) CronAgentOption {
	return func(c *cronConfig) {
		c.onJobProcessed = fn
	}
}

// WithDisableJobProcessing 显式关闭任务触发后的模型处理。
// 关闭后任务触发只执行 OnTriggered/OnProcessed 回调（response 为空），
// 不会调用模型。不要通过“是否设置回调”来推断是否需要模型处理。
func WithDisableJobProcessing() CronAgentOption {
	return func(c *cronConfig) {
		c.disableJobProcessing = true
	}
}

// WithJobContextBuilder 设置任务执行 context 构建器，
// 用于把 trace、超时或租户信息带进任务回调与模型调用。
func WithJobContextBuilder(builder JobContextBuilder) CronAgentOption {
	return func(c *cronConfig) {
		c.jobCtxBuilder = builder
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
func WithCronMiddlewares(mw ...adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]) CronAgentOption {
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

// WithCronJobTimeout caps the model call used to process triggered jobs.
func WithCronJobTimeout(timeout time.Duration) CronAgentOption {
	return func(c *cronConfig) {
		c.jobTimeout = timeout
	}
}

// CronAgentResult 定时任务 Agent 创建结果
type CronAgentResult struct {
	Agent   adk.TypedAgent[*schema.AgenticMessage]
	Service *CronService
}

type cronServiceProvider interface {
	CronService() *CronService
}

// cronLifecycleProvider 由 cron 工具实现，用于把工具层配置的回调
// 合并进 NewCronAgent 安装的任务处理器。
type cronLifecycleProvider interface {
	CronLifecycle() *JobLifecycle
}

// NewCronAgent 创建定时任务 Agent
// tools 参数由调用方通过 cronTool.GetTools(service) 提供，避免循环导入
func NewCronAgent(ctx context.Context, cm model.AgenticModel, cronTools []tool.BaseTool, opts ...CronAgentOption) (*CronAgentResult, error) {
	cfg := &cronConfig{
		maxJobsPerUser: defaultMaxJobsPerUser,
		jobTimeout:     time.Minute,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.storeErr != nil {
		return nil, fmt.Errorf("failed to load cron store: %w", cfg.storeErr)
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
	if cfg.jobCtxBuilder != nil {
		service.SetJobContextBuilder(cfg.jobCtxBuilder)
	}

	// 合并工具
	allTools := make([]tool.BaseTool, 0, len(cronTools)+len(cfg.extraTools))
	allTools = append(allTools, cronTools...)
	allTools = append(allTools, cfg.extraTools...)

	// 构建 Middleware 链
	handlers := make([]adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage], 0, len(cfg.middlewares)+1)
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

	agent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
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

	// 安装任务处理器。触发回调与处理回调是纯观测点，模型处理默认始终执行，
	// 三者不再互斥；需要关闭模型处理时显式传入 WithDisableJobProcessing。
	// 工具层（cronTool.GetTools）配置的回调先注册，Agent 层配置的回调后注册。
	var onTriggered []func(ctx context.Context, job *CronJob)
	var onProcessed []func(ctx context.Context, job *CronJob, response string, err error)
	for _, provider := range collectCronLifecycles(cronTools) {
		if provider.OnTriggered != nil {
			onTriggered = append(onTriggered, provider.OnTriggered)
		}
		if provider.OnProcessed != nil {
			onProcessed = append(onProcessed, provider.OnProcessed)
		}
	}
	if cfg.onJobTriggered != nil {
		onTriggered = append(onTriggered, cfg.onJobTriggered)
	}
	if cfg.onJobProcessed != nil {
		onProcessed = append(onProcessed, cfg.onJobProcessed)
	}

	jobTimeout := cfg.jobTimeout
	disableProcessing := cfg.disableJobProcessing

	service.SetOnJobContext(func(ctx context.Context, job *CronJob) (string, error) {
		for _, fn := range onTriggered {
			fn(ctx, job)
		}

		if disableProcessing {
			const skipped = "skipped: job processing disabled"
			for _, fn := range onProcessed {
				fn(ctx, job, skipped, nil)
			}
			return skipped, nil
		}

		var jobCtx context.Context
		var cancel context.CancelFunc
		if jobTimeout > 0 {
			jobCtx, cancel = context.WithTimeout(ctx, jobTimeout)
		} else {
			jobCtx, cancel = context.WithCancel(ctx)
		}
		defer cancel()

		resp, err := cm.Generate(jobCtx, []*schema.AgenticMessage{
			schema.SystemAgenticMessage("你是一个提醒助手。请将以下定时任务消息转换为简洁、友好的提醒通知。直接输出一句话。"),
			schema.UserAgenticMessage(job.Payload.Message),
		})

		var response string
		if resp != nil {
			response = agmsg.Text(resp)
		}
		for _, fn := range onProcessed {
			fn(ctx, job, response, err)
		}
		return response, err
	})

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

func collectCronLifecycles(cronTools []tool.BaseTool) []*JobLifecycle {
	var lifecycles []*JobLifecycle
	for _, cronTool := range cronTools {
		provider, ok := cronTool.(cronLifecycleProvider)
		if !ok {
			continue
		}
		if lc := provider.CronLifecycle(); lc != nil {
			lifecycles = append(lifecycles, lc)
		}
	}
	return lifecycles
}

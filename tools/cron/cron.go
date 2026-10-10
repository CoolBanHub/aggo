package cron

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	cronPkg "github.com/CoolBanHub/aggo/cron"
	"github.com/CoolBanHub/aggo/pkg/sessionkeys"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// CronOption 配置选项
type CronOption func(*CronTool)

// WithOnJobTriggered 设置任务触发时的回调（观测用，不拦截模型处理）。
// 注意：如果同时使用 cronPkg.NewCronAgent，Agent 会安装自己的任务处理器，
// 该回调会被合并进 Agent 的处理器中执行。
func WithOnJobTriggered(fn func(job *cronPkg.CronJob)) CronOption {
	if fn == nil {
		return func(t *CronTool) {}
	}
	return func(t *CronTool) {
		t.onJobTriggered = func(ctx context.Context, job *cronPkg.CronJob) {
			fn(job)
		}
	}
}

// WithOnJobProcessed 设置任务处理完成后的回调（观测用）。
// 该回调只在独立使用本工具（未配合 cronPkg.NewCronAgent）时不会被自动触发；
// 配合 NewCronAgent 时请使用 cronPkg.WithOnJobProcessed。
func WithOnJobProcessed(fn func(job *cronPkg.CronJob, response string, err error)) CronOption {
	if fn == nil {
		return func(t *CronTool) {}
	}
	return func(t *CronTool) {
		t.onJobProcessed = func(ctx context.Context, job *cronPkg.CronJob, response string, err error) {
			fn(job, response, err)
		}
	}
}

// CronTool 定时任务工具
type CronTool struct {
	service        *cronPkg.CronService
	onJobTriggered func(ctx context.Context, job *cronPkg.CronJob)
	onJobProcessed func(ctx context.Context, job *cronPkg.CronJob, response string, err error)
}

type serviceBoundTool struct {
	tool.InvokableTool
	service *cronPkg.CronService
	ct      *CronTool
}

func (t *serviceBoundTool) CronService() *cronPkg.CronService {
	return t.service
}

// CronLifecycle 暴露工具层配置的任务回调，
// 供 cronPkg.NewCronAgent 合并进统一的任务处理器。
func (t *serviceBoundTool) CronLifecycle() *cronPkg.JobLifecycle {
	return &cronPkg.JobLifecycle{
		OnTriggered: t.ct.onJobTriggered,
		OnProcessed: t.ct.onJobProcessed,
	}
}

// CronParams 工具参数
type CronParams struct {
	// 操作类型
	Action string `json:"action" jsonschema:"description=操作类型,required,enum=add,enum=list,enum=remove,enum=enable,enum=disable"`

	// 定时任务的消息内容（add 时必填）
	Message string `json:"message,omitempty" jsonschema:"description=定时任务触发时的消息内容"`

	// 一次性定时：从现在开始的秒数后触发（如 600 表示 10 分钟后）
	AtSeconds *int64 `json:"at_seconds,omitempty" jsonschema:"description=一次性定时：从现在开始多少秒后触发"`

	// 周期定时：每隔多少秒触发一次（如 3600 表示每小时）
	EverySeconds *int64 `json:"every_seconds,omitempty" jsonschema:"description=周期定时：每隔多少秒触发一次"`

	// Cron 表达式（如 '0 9 * * *' 表示每天早上 9 点）
	CronExpr string `json:"cron_expr,omitempty" jsonschema:"description=Cron 表达式，用于复杂的周期调度"`

	// 任务 ID（remove/enable/disable 时必填）
	JobID string `json:"job_id,omitempty" jsonschema:"description=任务 ID（用于 remove/enable/disable 操作）"`
}

// GetTools 从已有的 CronService 获取定时任务工具列表
func GetTools(service *cronPkg.CronService, opts ...CronOption) []tool.BaseTool {
	ct := &CronTool{
		service: service,
	}

	for _, opt := range opts {
		opt(ct)
	}

	// 独立使用本工具（未配合 NewCronAgent）时，安装只含观测回调的处理器，
	// 保持旧行为可用；NewCronAgent 会用自己的处理器覆盖并合并这些回调。
	if ct.onJobTriggered != nil || ct.onJobProcessed != nil {
		onTriggered := ct.onJobTriggered
		onProcessed := ct.onJobProcessed
		service.SetOnJobContext(func(ctx context.Context, job *cronPkg.CronJob) (string, error) {
			if onTriggered != nil {
				onTriggered(ctx, job)
			}
			if onProcessed != nil {
				onProcessed(ctx, job, "", nil)
			}
			return "ok", nil
		})
	}

	name := "cron"
	desc := "定时任务调度工具。支持添加、查看、删除、启用和禁用当前用户的定时任务。" +
		"使用 'at_seconds' 设置一次性定时（如：提醒我10分钟后 → at_seconds=600）；" +
		"使用 'every_seconds' 设置周期定时（如：每2小时 → every_seconds=7200）；" +
		"使用 'cron_expr' 设置 Cron 表达式调度（如：每天9点 → cron_expr='0 9 * * *'）。" +
		"列表与变更操作只作用于当前会话身份所属的任务。"

	t, _ := utils.InferTool(name, desc, ct.execute)

	return []tool.BaseTool{&serviceBoundTool{
		InvokableTool: t,
		service:       service,
		ct:            ct,
	}}
}

func (ct *CronTool) execute(ctx context.Context, params CronParams) (interface{}, error) {
	// 归属身份从运行时会话解析，模型不可通过参数指定；
	// 解析不到身份时拒绝执行，不能退化为空字符串（否则会看到/操作所有人的任务）。
	ownerID, err := requireOwner(ctx)
	if err != nil {
		return nil, err
	}

	switch params.Action {
	case "add":
		return ct.addJob(ctx, params, ownerID)
	case "list":
		return ct.listJobs(ownerID)
	case "remove":
		return ct.removeJob(params, ownerID)
	case "enable":
		return ct.enableJob(params, ownerID, true)
	case "disable":
		return ct.enableJob(params, ownerID, false)
	default:
		return nil, fmt.Errorf("unknown action: %s", params.Action)
	}
}

func requireOwner(ctx context.Context) (string, error) {
	if id, ok := sessionkeys.ResolveOwner(ctx); ok {
		return id, nil
	}
	return "", fmt.Errorf("cron: 无法从会话中解析用户身份，拒绝执行该操作。" +
		"请通过 adk.WithSessionValues 注入 userID/actorID（或会话级 sessionID）后重试")
}

func (ct *CronTool) addJob(ctx context.Context, params CronParams, ownerID string) (interface{}, error) {
	if params.Message == "" {
		return nil, fmt.Errorf("message is required for add action")
	}

	var schedule cronPkg.CronSchedule

	switch {
	case params.AtSeconds != nil:
		atMS := time.Now().UnixMilli() + *params.AtSeconds*1000
		schedule = cronPkg.CronSchedule{
			Kind: "at",
			AtMS: &atMS,
		}
	case params.EverySeconds != nil:
		everyMS := *params.EverySeconds * 1000
		schedule = cronPkg.CronSchedule{
			Kind:    "every",
			EveryMS: &everyMS,
		}
	case params.CronExpr != "":
		schedule = cronPkg.CronSchedule{
			Kind: "cron",
			Expr: params.CronExpr,
		}
	default:
		return nil, fmt.Errorf("one of at_seconds, every_seconds, or cron_expr is required")
	}

	// 截断 message 作为任务名（按字符而非字节截断，避免中文乱码）
	name := params.Message
	nameRunes := []rune(name)
	if len(nameRunes) > 30 {
		name = string(nameRunes[:30]) + "..."
	}

	job, err := ct.service.AddJob(name, schedule, params.Message, ownerID)
	if err != nil {
		return nil, fmt.Errorf("error adding job: %w", err)
	}

	return map[string]interface{}{
		"operation": "add",
		"success":   true,
		"job_id":    job.ID,
		"job_name":  job.Name,
		"message":   fmt.Sprintf("定时任务已添加: %s (id: %s)", job.Name, job.ID),
	}, nil
}

func (ct *CronTool) listJobs(ownerID string) (interface{}, error) {
	jobs, err := ct.service.ListJobsByUser(ownerID, true)
	if err != nil {
		if errors.Is(err, cronPkg.ErrJobNotFound) {
			return nil, fmt.Errorf("cron: 无法从会话中解析用户身份，拒绝列出定时任务")
		}
		return nil, fmt.Errorf("error listing jobs: %w", err)
	}

	if len(jobs) == 0 {
		return map[string]interface{}{
			"operation": "list",
			"success":   true,
			"jobs":      []interface{}{},
			"message":   "当前没有定时任务",
		}, nil
	}

	var jobList []map[string]interface{}
	for _, j := range jobs {
		var scheduleInfo string
		switch j.Schedule.Kind {
		case "every":
			if j.Schedule.EveryMS != nil {
				scheduleInfo = fmt.Sprintf("every %ds", *j.Schedule.EveryMS/1000)
			}
		case "cron":
			scheduleInfo = j.Schedule.Expr
		case "at":
			scheduleInfo = "one-time"
		default:
			scheduleInfo = "unknown"
		}

		jobList = append(jobList, map[string]interface{}{
			"id":       j.ID,
			"name":     j.Name,
			"enabled":  j.Enabled,
			"schedule": scheduleInfo,
			"message":  j.Payload.Message,
		})
	}

	var result strings.Builder
	result.WriteString("定时任务列表:\n")
	for _, j := range jobs {
		status := "✓"
		if !j.Enabled {
			status = "✗"
		}
		result.WriteString(fmt.Sprintf("- [%s] %s (id: %s)\n", status, j.Name, j.ID))
	}

	return map[string]interface{}{
		"operation": "list",
		"success":   true,
		"jobs":      jobList,
		"message":   result.String(),
	}, nil
}

func (ct *CronTool) removeJob(params CronParams, ownerID string) (interface{}, error) {
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required for remove action")
	}

	if err := ct.service.RemoveJobForUser(params.JobID, ownerID); err != nil {
		if errors.Is(err, cronPkg.ErrJobNotFound) {
			return nil, fmt.Errorf("job %s not found", params.JobID)
		}
		return nil, fmt.Errorf("error removing job: %w", err)
	}

	return map[string]interface{}{
		"operation": "remove",
		"success":   true,
		"job_id":    params.JobID,
		"message":   fmt.Sprintf("定时任务已删除: %s", params.JobID),
	}, nil
}

func (ct *CronTool) enableJob(params CronParams, ownerID string, enable bool) (interface{}, error) {
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required for enable/disable action")
	}

	job, err := ct.service.EnableJobForUser(params.JobID, ownerID, enable)
	if err != nil {
		if errors.Is(err, cronPkg.ErrJobNotFound) {
			return nil, fmt.Errorf("job %s not found", params.JobID)
		}
		return nil, fmt.Errorf("error updating job: %w", err)
	}

	action := "启用"
	if !enable {
		action = "禁用"
	}

	return map[string]interface{}{
		"operation": "enable",
		"success":   true,
		"job_id":    job.ID,
		"job_name":  job.Name,
		"enabled":   enable,
		"message":   fmt.Sprintf("定时任务 '%s' 已%s", job.Name, action),
	}, nil
}

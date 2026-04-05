package cron

import (
	"context"
	"fmt"
	"strings"
	"time"

	cronPkg "github.com/CoolBanHub/aggo/cron"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// CronOption 配置选项
type CronOption func(*CronTool)

// WithOnJobTriggered 设置任务触发时的回调
func WithOnJobTriggered(fn func(job *cronPkg.CronJob)) CronOption {
	return func(t *CronTool) {
		t.onJobTriggered = fn
	}
}

// CronTool 定时任务工具
type CronTool struct {
	service        *cronPkg.CronService
	onJobTriggered func(job *cronPkg.CronJob)
}

type serviceBoundTool struct {
	tool.InvokableTool
	service *cronPkg.CronService
}

func (t *serviceBoundTool) CronService() *cronPkg.CronService {
	return t.service
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

	// 设置 service 的回调
	service.SetOnJob(func(job *cronPkg.CronJob) (string, error) {
		if ct.onJobTriggered != nil {
			ct.onJobTriggered(job)
		}
		return "ok", nil
	})

	name := "cron"
	desc := "定时任务调度工具。支持添加、查看、删除、启用和禁用定时任务。" +
		"使用 'at_seconds' 设置一次性定时（如：提醒我10分钟后 → at_seconds=600）；" +
		"使用 'every_seconds' 设置周期定时（如：每2小时 → every_seconds=7200）；" +
		"使用 'cron_expr' 设置 Cron 表达式调度（如：每天9点 → cron_expr='0 9 * * *'）。"

	t, _ := utils.InferTool(name, desc, ct.execute)

	return []tool.BaseTool{&serviceBoundTool{
		InvokableTool: t,
		service:       service,
	}}
}

func (ct *CronTool) execute(ctx context.Context, params CronParams) (interface{}, error) {
	switch params.Action {
	case "add":
		return ct.addJob(ctx, params)
	case "list":
		return ct.listJobs()
	case "remove":
		return ct.removeJob(params)
	case "enable":
		return ct.enableJob(params, true)
	case "disable":
		return ct.enableJob(params, false)
	default:
		return nil, fmt.Errorf("unknown action: %s", params.Action)
	}
}

func (ct *CronTool) addJob(ctx context.Context, params CronParams) (interface{}, error) {
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

	var userID string
	if v, ok := adk.GetSessionValue(ctx, "userID"); ok {
		if s, ok := v.(string); ok {
			userID = s
		}
	}
	if userID == "" {
		if v, ok := adk.GetSessionValue(ctx, "sessionID"); ok {
			if s, ok := v.(string); ok {
				userID = s
			}
		}
	}
	job, err := ct.service.AddJob(name, schedule, params.Message, userID)
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

func (ct *CronTool) listJobs() (interface{}, error) {
	jobs := ct.service.ListJobs(true)

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

func (ct *CronTool) removeJob(params CronParams) (interface{}, error) {
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required for remove action")
	}

	if ct.service.RemoveJob(params.JobID) {
		return map[string]interface{}{
			"operation": "remove",
			"success":   true,
			"job_id":    params.JobID,
			"message":   fmt.Sprintf("定时任务已删除: %s", params.JobID),
		}, nil
	}
	return nil, fmt.Errorf("job %s not found", params.JobID)
}

func (ct *CronTool) enableJob(params CronParams, enable bool) (interface{}, error) {
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required for enable/disable action")
	}

	job := ct.service.EnableJob(params.JobID, enable)
	if job == nil {
		return nil, fmt.Errorf("job %s not found", params.JobID)
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

package cron

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

// JobHandler 任务触发回调
type JobHandler func(job *CronJob) (string, error)

// Locker 分布式锁接口，便于后续扩展
type Locker interface {
	Lock(ctx context.Context, key string) error
	Unlock(ctx context.Context, key string) error
}

// CronService 定时调度服务
type CronService struct {
	store          Store
	onJob          JobHandler
	mu             sync.RWMutex
	running        bool
	scheduler      gocron.Scheduler
	locker         Locker
	maxJobs        int // 任务数量上限，0 表示不限制
	maxJobsPerUser int // 单用户任务数量上限，0 表示不限制

	// jobIDToGocronID 维护内部 ID 到 gocron ID 的映射
	jobIDToGocronID map[string]uuid.UUID
}

// NewCronService 创建定时调度服务
func NewCronService(store Store, onJob JobHandler) *CronService {
	return &CronService{
		store:           store,
		onJob:           onJob,
		jobIDToGocronID: make(map[string]uuid.UUID),
	}
}

// SetLocker 设置分布式锁实现
func (cs *CronService) SetLocker(locker Locker) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.locker = locker
}

// SetMaxJobs 设置最大任务数量限制，0 表示不限制
func (cs *CronService) SetMaxJobs(max int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.maxJobs = max
}

// SetMaxJobsPerUser 设置单用户最大任务数量限制，0 表示不限制
func (cs *CronService) SetMaxJobsPerUser(max int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.maxJobsPerUser = max
}

// Start 启动调度循环
func (cs *CronService) Start() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.running {
		return nil
	}

	// 创建调度器
	s, err := gocron.NewScheduler()
	if err != nil {
		return fmt.Errorf("failed to create gocron scheduler: %w", err)
	}
	cs.scheduler = s

	// 加载并启动所有已启用的任务
	jobs, err := cs.store.List()
	if err != nil {
		return fmt.Errorf("failed to list jobs from store: %w", err)
	}

	for i := range jobs {
		job := &jobs[i]
		if job.Enabled {
			if err := cs.registerJobInScheduler(job); err != nil {
				log.Printf("[cron] failed to register job %s: %v", job.ID, err)
			}
		}
	}

	cs.scheduler.Start()
	cs.running = true

	return nil
}

// Stop 停止调度循环
func (cs *CronService) Stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.running {
		return
	}

	if cs.scheduler != nil {
		_ = cs.scheduler.Shutdown()
		cs.scheduler = nil
	}
	cs.running = false
	cs.jobIDToGocronID = make(map[string]uuid.UUID)
}

func (cs *CronService) registerJobInScheduler(job *CronJob) error {
	var definition gocron.JobDefinition

	switch job.Schedule.Kind {
	case "at":
		if job.Schedule.AtMS == nil {
			return fmt.Errorf("atMs is required for kind=at")
		}
		startTime := time.UnixMilli(*job.Schedule.AtMS)
		if startTime.Before(time.Now()) {
			// 如果时间已过，标记为禁用并跳过
			job.Enabled = false
			job.State.NextRunAtMS = nil
			_ = cs.store.Save(job)
			return nil
		}
		definition = gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(startTime))

	case "every":
		if job.Schedule.EveryMS == nil || *job.Schedule.EveryMS <= 0 {
			return fmt.Errorf("everyMs is required for kind=every")
		}
		definition = gocron.DurationJob(time.Duration(*job.Schedule.EveryMS) * time.Millisecond)

	case "cron":
		if job.Schedule.Expr == "" {
			return fmt.Errorf("expr is required for kind=cron")
		}
		definition = gocron.CronJob(job.Schedule.Expr, false)

	default:
		return fmt.Errorf("unknown schedule kind: %s", job.Schedule.Kind)
	}

	// 包装执行逻辑，处理分布式锁和状态更新
	task := func() {
		cs.executeJob(job.ID)
	}

	gJob, err := cs.scheduler.NewJob(definition, gocron.NewTask(task))
	if err != nil {
		return err
	}

	cs.jobIDToGocronID[job.ID] = gJob.ID()

	// 更新下次运行时间到 Store
	nextRun, _ := gJob.NextRun()
	if !nextRun.IsZero() {
		ms := nextRun.UnixMilli()
		job.State.NextRunAtMS = &ms
		_ = cs.store.Save(job)
	}

	return nil
}

func (cs *CronService) executeJob(jobID string) {
	// 获取最新的任务信息
	cs.mu.Lock()
	job, err := cs.store.Get(jobID)
	cs.mu.Unlock()
	if err != nil || !job.Enabled {
		return
	}

	// 分布式锁逻辑
	if cs.locker != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cs.locker.Lock(ctx, "cron_job_"+jobID); err != nil {
			log.Printf("[cron] failed to acquire lock for job %s: %v", jobID, err)
			return
		}
		defer cs.locker.Unlock(ctx, "cron_job_"+jobID)
	}

	startTime := time.Now().UnixMilli()

	var jobErr error
	if cs.onJob != nil {
		_, jobErr = cs.onJob(job)
	}

	// 更新状态
	cs.mu.Lock()
	defer cs.mu.Unlock()

	current, getErr := cs.store.Get(jobID)
	if getErr != nil {
		return
	}

	current.State.LastRunAtMS = &startTime
	current.UpdatedAtMS = time.Now().UnixMilli()

	if jobErr != nil {
		current.State.LastStatus = "error"
		current.State.LastError = jobErr.Error()
	} else {
		current.State.LastStatus = "ok"
		current.State.LastError = ""
	}

	// 处理一次性任务的后续逻辑
	if current.Schedule.Kind == "at" {
		if current.DeleteAfterRun {
			_ = cs.store.Delete(current.ID)
			if gid, ok := cs.jobIDToGocronID[current.ID]; ok && cs.scheduler != nil {
				_ = cs.scheduler.RemoveJob(gid)
				delete(cs.jobIDToGocronID, current.ID)
			}
			return
		}
		current.Enabled = false
		current.State.NextRunAtMS = nil
	} else {
		// 更新下次运行时间
		if gid, ok := cs.jobIDToGocronID[current.ID]; ok && cs.scheduler != nil {
			// 尝试从 gocron 获取真实下次运行时间
			for _, gj := range cs.scheduler.Jobs() {
				if gj.ID() == gid {
					next, _ := gj.NextRun()
					if !next.IsZero() {
						ms := next.UnixMilli()
						current.State.NextRunAtMS = &ms
					}
					break
				}
			}
		}
	}

	_ = cs.store.Save(current)
}

// AddJob 添加定时任务
func (cs *CronService) AddJob(name string, schedule CronSchedule, message string, userID string) (*CronJob, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// 检查任务数量上限
	jobs, err := cs.store.List()
	if err != nil {
		return nil, fmt.Errorf("failed to check job count: %w", err)
	}

	if cs.maxJobs > 0 && len(jobs) >= cs.maxJobs {
		return nil, fmt.Errorf("已达到任务总数上限 (%d)，请先删除旧任务", cs.maxJobs)
	}

	// 检查单用户任务数量上限
	if cs.maxJobsPerUser > 0 && userID != "" {
		var userJobCount int
		for _, j := range jobs {
			if j.UserID == userID {
				userJobCount++
			}
		}
		if userJobCount >= cs.maxJobsPerUser {
			return nil, fmt.Errorf("用户已达到任务数量上限 (%d)，请先删除旧任务", cs.maxJobsPerUser)
		}
	}

	now := time.Now().UnixMilli()
	deleteAfterRun := (schedule.Kind == "at")

	job := &CronJob{
		ID:       generateID(),
		UserID:   userID,
		Name:     name,
		Enabled:  true,
		Schedule: schedule,
		Payload: CronPayload{
			Message: message,
		},
		CreatedAtMS:    now,
		UpdatedAtMS:    now,
		DeleteAfterRun: deleteAfterRun,
	}

	if err := cs.store.Save(job); err != nil {
		return nil, err
	}

	if cs.running {
		if err := cs.registerJobInScheduler(job); err != nil {
			// 即使注册失败也返回 job，因为已经存入 store，下次启动会重试
			log.Printf("[cron] failed to register new job in scheduler: %v", err)
		}
	}

	return job, nil
}

// RemoveJob 删除任务
func (cs *CronService) RemoveJob(jobID string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if gid, ok := cs.jobIDToGocronID[jobID]; ok && cs.scheduler != nil {
		_ = cs.scheduler.RemoveJob(gid)
		delete(cs.jobIDToGocronID, jobID)
	}

	if err := cs.store.Delete(jobID); err != nil {
		return false
	}
	return true
}

// EnableJob 启用/禁用任务
func (cs *CronService) EnableJob(jobID string, enabled bool) *CronJob {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	job, err := cs.store.Get(jobID)
	if err != nil {
		return nil
	}

	if job.Enabled == enabled {
		return job
	}

	job.Enabled = enabled
	job.UpdatedAtMS = time.Now().UnixMilli()

	if enabled {
		if cs.running {
			if err := cs.registerJobInScheduler(job); err != nil {
				log.Printf("[cron] failed to enable job %s: %v", jobID, err)
			}
		}
	} else {
		if gid, ok := cs.jobIDToGocronID[jobID]; ok && cs.scheduler != nil {
			_ = cs.scheduler.RemoveJob(gid)
			delete(cs.jobIDToGocronID, jobID)
		}
		job.State.NextRunAtMS = nil
	}

	if err := cs.store.Save(job); err != nil {
		log.Printf("[cron] failed to save job after enable: %v", err)
	}
	return job
}

// ListJobs 列出任务
func (cs *CronService) ListJobs(includeDisabled bool) []CronJob {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	jobs, err := cs.store.List()
	if err != nil {
		log.Printf("[cron] failed to list jobs: %v", err)
		return nil
	}

	if includeDisabled {
		return jobs
	}

	var enabled []CronJob
	for _, job := range jobs {
		if job.Enabled {
			enabled = append(enabled, job)
		}
	}
	return enabled
}

// Status 返回服务状态
func (cs *CronService) Status() map[string]any {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	jobs, _ := cs.store.List()
	var enabledCount int
	for _, job := range jobs {
		if job.Enabled {
			enabledCount++
		}
	}

	return map[string]any{
		"running":     cs.running,
		"totalJobs":   len(jobs),
		"enabledJobs": enabledCount,
	}
}

// SetOnJob 设置任务触发回调
func (cs *CronService) SetOnJob(handler JobHandler) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.onJob = handler
}

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// 辅助方法：解析 UUID（处理 gocron 的 ID 类型）
func parseUUID(s string) uuid.UUID {
	// gocron.JobID is uuid.UUID
	id, err := uuid.Parse(s)
	if err != nil {
		log.Printf("[cron] failed to parse UUID string '%s': %v", s, err)
		return uuid.Nil // Return a nil UUID on error
	}
	return id
}

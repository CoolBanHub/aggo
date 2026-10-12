package cron

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

// ErrJobNotFound 表示任务不存在，或任务存在但不属于当前调用者。
// 两种情况返回同一个错误，避免向未授权调用者泄漏任务是否存在。
var ErrJobNotFound = errors.New("cron: job not found or not owned by caller")

// Locker 分布式锁接口，便于后续扩展
type Locker interface {
	Lock(ctx context.Context, key string) error
	Unlock(ctx context.Context, key string) error
}

const (
	defaultLockTimeout   = 10 * time.Second
	defaultUnlockTimeout = 10 * time.Second
)

// JobHandler 任务触发回调（兼容旧签名）。
// 新代码建议使用 ContextJobHandler，以便关联任务上下文与 trace。
type JobHandler func(job *CronJob) (string, error)

// ContextJobHandler 带任务上下文的触发处理器。
// ctx 由 JobContextBuilder 构建（默认注入任务 OwnerID 与 JobID）。
type ContextJobHandler func(ctx context.Context, job *CronJob) (string, error)

// JobContextBuilder 为单次任务执行构建 context。
// 返回 nil 时回退到默认 context。builder 不应修改任务本身。
type JobContextBuilder func(ctx context.Context, job *CronJob) context.Context

type jobContextKey string

const (
	jobContextKeyOwnerID jobContextKey = "aggo.cron.jobOwnerID"
	jobContextKeyJobID   jobContextKey = "aggo.cron.jobID"
)

// WithJobContext 向 ctx 注入任务身份信息。
func WithJobContext(ctx context.Context, job *CronJob) context.Context {
	if job == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, jobContextKeyOwnerID, job.UserID)
	ctx = context.WithValue(ctx, jobContextKeyJobID, job.ID)
	return ctx
}

// JobOwnerIDFromContext 读取任务归属者 ID。
func JobOwnerIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(jobContextKeyOwnerID).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// JobIDFromContext 读取任务 ID。
func JobIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(jobContextKeyJobID).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// CronService 定时调度服务
type CronService struct {
	store Store
	onJob ContextJobHandler
	mu    sync.RWMutex
	// running 由 mu 保护
	running        bool
	stopping       bool
	scheduler      gocron.Scheduler
	locker         Locker
	lockTimeout    time.Duration
	unlockTimeout  time.Duration
	maxJobs        int // 任务数量上限，0 表示不限制
	maxJobsPerUser int // 单用户任务数量上限，0 表示不限制

	// jobIDToGocronID 维护内部 ID 到 gocron ID 的映射
	jobIDToGocronID map[string]uuid.UUID

	// jobCtxBuilder 由 mu 保护
	jobCtxBuilder JobContextBuilder

	// jobWg 跟踪正在执行的任务回调，StopContext 等待其结束
	jobWg sync.WaitGroup
}

// NewCronService 创建定时调度服务
func NewCronService(store Store, onJob JobHandler) *CronService {
	cs := &CronService{
		store:           store,
		jobIDToGocronID: make(map[string]uuid.UUID),
		lockTimeout:     defaultLockTimeout,
		unlockTimeout:   defaultUnlockTimeout,
	}
	if onJob != nil {
		cs.SetOnJob(onJob)
	}
	return cs
}

// SetLocker 设置分布式锁实现
func (cs *CronService) SetLocker(locker Locker) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.locker = locker
}

// SetLockTimeouts 设置获取/释放分布式锁的超时。
// 两者独立计时：任务执行超过 lockTimeout 后，解锁仍使用新的有效 context。
func (cs *CronService) SetLockTimeouts(lock, unlock time.Duration) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if lock > 0 {
		cs.lockTimeout = lock
	}
	if unlock > 0 {
		cs.unlockTimeout = unlock
	}
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

// SetJobContextBuilder 设置任务执行 context 构建器。
// builder 返回的 context 会注入任务 OwnerID 与 JobID 后传给任务回调。
func (cs *CronService) SetJobContextBuilder(builder JobContextBuilder) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.jobCtxBuilder = builder
}

// SetOnJob 设置任务触发回调（兼容旧签名，不携带任务上下文）
func (cs *CronService) SetOnJob(handler JobHandler) {
	if handler == nil {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		cs.onJob = nil
		return
	}
	cs.SetOnJobContext(func(ctx context.Context, job *CronJob) (string, error) {
		return handler(job)
	})
}

// SetOnJobContext 设置带任务上下文的触发回调
func (cs *CronService) SetOnJobContext(handler ContextJobHandler) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.onJob = handler
}

// Start 启动调度循环
func (cs *CronService) Start() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.running {
		return nil
	}
	if cs.stopping {
		return errors.New("cron: service is still stopping")
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

	var startErrs []error
	for i := range jobs {
		job := &jobs[i]
		if !job.Enabled {
			continue
		}
		if err := cs.registerJobInScheduler(job); err != nil {
			log.Printf("[cron] failed to register job %s: %v", job.ID, err)
			// 标记为不可用并持久化，避免每次启动都在同一个任务上失败。
			// 持久化失败意味着内存与存储状态分裂，必须让调用方看到。
			job.Enabled = false
			if saveErr := cs.store.Save(job); saveErr != nil {
				startErrs = append(startErrs, fmt.Errorf("job %s: register failed: %v, persist disable failed: %w", job.ID, err, saveErr))
			}
		}
	}

	cs.scheduler.Start()
	cs.running = true

	if len(startErrs) > 0 {
		return fmt.Errorf("failed to start cron service: %w", errors.Join(startErrs...))
	}

	return nil
}

// Stop 停止调度循环，并等待正在执行的任务结束
func (cs *CronService) Stop() {
	_ = cs.StopContext(context.Background())
}

// StopContext 停止调度循环，并最多等待 ctx 到期让正在执行的任务退出。
// 返回等待超时错误时，任务回调可能仍在后台执行。
func (cs *CronService) StopContext(ctx context.Context) error {
	cs.mu.Lock()
	var s gocron.Scheduler
	if cs.running {
		s = cs.scheduler
		cs.scheduler = nil
		cs.running = false
		cs.stopping = true
		cs.jobIDToGocronID = make(map[string]uuid.UUID)
	}
	if s == nil {
		// Prevent callbacks that raced with a previous scheduler shutdown from
		// registering themselves while Stop waits for the current executions.
		cs.stopping = true
	}
	cs.mu.Unlock()

	var shutdownErr error
	if s != nil {
		shutdownErr = s.Shutdown()
	}

	// 即便未处于 running 状态，也等待仍在执行的任务回调结束
	done := make(chan struct{})
	go func() {
		cs.jobWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		cs.mu.Lock()
		cs.stopping = false
		cs.mu.Unlock()
		return shutdownErr
	case <-ctx.Done():
		return fmt.Errorf("cron: timed out waiting for running jobs: %w (scheduler shutdown: %v)", ctx.Err(), shutdownErr)
	}
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
			if err := cs.store.Save(job); err != nil {
				return fmt.Errorf("failed to persist expired job state: %w", err)
			}
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
		if err := cs.store.Save(job); err != nil {
			_ = cs.scheduler.RemoveJob(gJob.ID())
			delete(cs.jobIDToGocronID, job.ID)
			return fmt.Errorf("failed to persist next run time: %w", err)
		}
	}

	return nil
}

func (cs *CronService) executeJob(jobID string) {
	cs.mu.Lock()
	job, err := cs.store.Get(jobID)
	handler := cs.onJob
	locker := cs.locker
	lockTimeout := cs.lockTimeout
	unlockTimeout := cs.unlockTimeout
	jobCtxBuilder := cs.jobCtxBuilder
	if err == nil && job != nil && job.Enabled && !cs.stopping {
		// Register the callback while holding the same lock Stop uses to enter
		// its waiting phase. A callback that starts after stopping is rejected.
		cs.jobWg.Add(1)
	} else {
		cs.mu.Unlock()
		if err != nil {
			log.Printf("[cron] failed to load job %s for execution: %v", jobID, err)
		}
		return
	}
	cs.mu.Unlock()

	jobCtx := context.Background()
	if jobCtxBuilder != nil {
		if built := jobCtxBuilder(jobCtx, job); built != nil {
			jobCtx = built
		}
	}
	jobCtx = WithJobContext(jobCtx, job)

	// 跟踪整个执行过程（含锁的获取/释放），StopContext 等待其结束
	defer cs.jobWg.Done()

	// 分布式锁：获取与释放使用独立的 context 和超时，
	// 避免任务执行超过锁超时后用已取消的 context 解锁。
	if locker != nil {
		lockKey := "cron_job_" + jobID
		lockCtx, cancelLock := context.WithTimeout(context.Background(), lockTimeout)
		if err := locker.Lock(lockCtx, lockKey); err != nil {
			cancelLock()
			log.Printf("[cron] failed to acquire lock for job %s: %v", jobID, err)
			return
		}
		cancelLock()
		defer func() {
			unlockCtx, cancelUnlock := context.WithTimeout(context.Background(), unlockTimeout)
			defer cancelUnlock()
			if err := locker.Unlock(unlockCtx, lockKey); err != nil {
				log.Printf("[cron] failed to unlock job %s: %v", jobID, err)
			}
		}()
	}

	startTime := time.Now().UnixMilli()

	var jobErr error
	if handler != nil {
		_, jobErr = handler(jobCtx, job)
	}

	cs.updateJobStateAfterRun(jobID, startTime, jobErr)
}

func (cs *CronService) updateJobStateAfterRun(jobID string, startTimeMS int64, jobErr error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	current, getErr := cs.store.Get(jobID)
	if getErr != nil {
		log.Printf("[cron] failed to reload job %s after run: %v", jobID, getErr)
		return
	}

	current.State.LastRunAtMS = &startTimeMS
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
			if err := cs.store.Delete(current.ID); err != nil {
				log.Printf("[cron] failed to delete finished one-shot job %s: %v", current.ID, err)
			} else if gid, ok := cs.jobIDToGocronID[current.ID]; ok && cs.scheduler != nil {
				if removeErr := cs.scheduler.RemoveJob(gid); removeErr != nil {
					log.Printf("[cron] failed to remove finished scheduler job %s: %v", current.ID, removeErr)
				}
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

	if err := cs.store.Save(current); err != nil {
		log.Printf("[cron] failed to persist job state after run %s: %v", jobID, err)
	}
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

// RemoveJobForUser 删除 userID 拥有的任务。
// userID 为空、任务不存在或任务属于其他用户时都返回 ErrJobNotFound。
func (cs *CronService) RemoveJobForUser(jobID, userID string) error {
	if userID == "" {
		return ErrJobNotFound
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	job, err := cs.store.Get(jobID)
	if err != nil || job == nil || job.UserID != userID {
		return ErrJobNotFound
	}

	if err := cs.store.Delete(jobID); err != nil {
		return fmt.Errorf("failed to delete job %s from store: %w", jobID, err)
	}
	// Persist first. If scheduler removal fails after this point, the callback
	// reloads the store and becomes a no-op instead of resurrecting deleted data.
	if gid, ok := cs.jobIDToGocronID[jobID]; ok && cs.scheduler != nil {
		if err := cs.scheduler.RemoveJob(gid); err != nil {
			log.Printf("[cron] failed to remove scheduler job %s after store deletion: %v", jobID, err)
		}
		delete(cs.jobIDToGocronID, jobID)
	}
	return nil
}

// RemoveJob 删除任务（不校验归属）。
//
// 管理员接口：仅供后台管理/运维场景使用；面向普通用户的调用请使用
// RemoveJobForUser，避免横向越权删除其他用户的任务。
//
// Deprecated: 该方法无法验证调用者身份，将在下一个主版本移除。
func (cs *CronService) RemoveJob(jobID string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if err := cs.store.Delete(jobID); err != nil {
		return false
	}
	if gid, ok := cs.jobIDToGocronID[jobID]; ok && cs.scheduler != nil {
		if err := cs.scheduler.RemoveJob(gid); err != nil {
			log.Printf("[cron] failed to remove scheduler job %s after store deletion: %v", jobID, err)
		}
		delete(cs.jobIDToGocronID, jobID)
	}
	return true
}

// EnableJobForUser 启用/禁用 userID 拥有的任务。
// userID 为空、任务不存在或任务属于其他用户时返回 ErrJobNotFound；
// 状态持久化失败时返回错误。
func (cs *CronService) EnableJobForUser(jobID, userID string, enabled bool) (*CronJob, error) {
	if userID == "" {
		return nil, ErrJobNotFound
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	job, err := cs.store.Get(jobID)
	if err != nil || job == nil || job.UserID != userID {
		return nil, ErrJobNotFound
	}

	if job.Enabled == enabled {
		return cloneJob(job), nil
	}

	job.Enabled = enabled
	job.UpdatedAtMS = time.Now().UnixMilli()

	if !enabled {
		job.State.NextRunAtMS = nil
	}
	// Save the authoritative state before changing the scheduler. A failed
	// write leaves the old scheduler state intact.
	if err := cs.store.Save(job); err != nil {
		return nil, fmt.Errorf("failed to save job after enable: %w", err)
	}
	if enabled {
		if cs.running {
			if err := cs.registerJobInScheduler(job); err != nil {
				// Roll back persistence so a failed registration cannot leave an
				// enabled job that is absent from the scheduler.
				job.Enabled = false
				_ = cs.store.Save(job)
				return nil, fmt.Errorf("failed to enable job %s: %w", jobID, err)
			}
		}
	} else {
		if gid, ok := cs.jobIDToGocronID[jobID]; ok && cs.scheduler != nil {
			if err := cs.scheduler.RemoveJob(gid); err != nil {
				log.Printf("[cron] failed to remove disabled scheduler job %s: %v", jobID, err)
			}
			delete(cs.jobIDToGocronID, jobID)
		}
	}
	return cloneJob(job), nil
}

// EnableJob 启用/禁用任务（不校验归属）。
//
// 管理员接口：仅供后台管理/运维场景使用；面向普通用户的调用请使用
// EnableJobForUser。
//
// Deprecated: 该方法无法验证调用者身份，将在下一个主版本移除。
func (cs *CronService) EnableJob(jobID string, enabled bool) *CronJob {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	job, err := cs.store.Get(jobID)
	if err != nil {
		return nil
	}

	if job.Enabled == enabled {
		return cloneJob(job)
	}

	job.Enabled = enabled
	job.UpdatedAtMS = time.Now().UnixMilli()

	if !enabled {
		job.State.NextRunAtMS = nil
	}
	if err := cs.store.Save(job); err != nil {
		log.Printf("[cron] failed to save job after enable: %v", err)
		return nil
	}
	if enabled {
		if cs.running {
			if err := cs.registerJobInScheduler(job); err != nil {
				log.Printf("[cron] failed to enable job %s: %v", jobID, err)
				job.Enabled = false
				_ = cs.store.Save(job)
			}
		}
	} else {
		if gid, ok := cs.jobIDToGocronID[jobID]; ok && cs.scheduler != nil {
			if err := cs.scheduler.RemoveJob(gid); err != nil {
				log.Printf("[cron] failed to remove disabled scheduler job %s: %v", jobID, err)
			}
			delete(cs.jobIDToGocronID, jobID)
		}
	}
	return cloneJob(job)
}

// ListJobsByUser 列出 userID 拥有的任务。userID 为空时返回错误，
// 不会退化为返回全量任务。
func (cs *CronService) ListJobsByUser(userID string, includeDisabled bool) ([]CronJob, error) {
	if userID == "" {
		return nil, fmt.Errorf("cron: userID is required to list jobs")
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()

	jobs, err := cs.store.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs from store: %w", err)
	}

	owned := make([]CronJob, 0, len(jobs))
	for _, job := range jobs {
		if job.UserID != userID {
			continue
		}
		if !includeDisabled && !job.Enabled {
			continue
		}
		owned = append(owned, job)
	}
	return owned, nil
}

// ListJobs 列出所有任务（不校验归属）。
//
// 管理员接口：仅供后台管理/运维场景使用；面向普通用户的调用请使用
// ListJobsByUser。
//
// Deprecated: 该方法会把所有用户的任务返回给调用方，将在下一个主版本移除。
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

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// cloneJob 深拷贝任务，避免调用方通过共享指针修改存储内部状态。
func cloneJob(job *CronJob) *CronJob {
	if job == nil {
		return nil
	}
	cloned := *job
	if job.Schedule.AtMS != nil {
		v := *job.Schedule.AtMS
		cloned.Schedule.AtMS = &v
	}
	if job.Schedule.EveryMS != nil {
		v := *job.Schedule.EveryMS
		cloned.Schedule.EveryMS = &v
	}
	if job.State.NextRunAtMS != nil {
		v := *job.State.NextRunAtMS
		cloned.State.NextRunAtMS = &v
	}
	if job.State.LastRunAtMS != nil {
		v := *job.State.LastRunAtMS
		cloned.State.LastRunAtMS = &v
	}
	return &cloned
}

package cron

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/adhocore/gronx"
)

// JobHandler 任务触发回调
type JobHandler func(job *CronJob) (string, error)

// CronService 定时调度服务
type CronService struct {
	store    Store
	onJob    JobHandler
	mu       sync.RWMutex
	running  bool
	stopChan chan struct{}
	gronx    *gronx.Gronx
}

// NewCronService 创建定时调度服务
func NewCronService(store Store, onJob JobHandler) *CronService {
	return &CronService{
		store: store,
		onJob: onJob,
		gronx: gronx.New(),
	}
}

// Start 启动调度循环
func (cs *CronService) Start() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.running {
		return nil
	}

	// 重新计算所有任务的下次运行时间
	if err := cs.recomputeNextRuns(); err != nil {
		return fmt.Errorf("failed to recompute next runs: %w", err)
	}

	cs.stopChan = make(chan struct{})
	cs.running = true
	go cs.runLoop(cs.stopChan)

	return nil
}

// Stop 停止调度循环
func (cs *CronService) Stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.running {
		return
	}

	cs.running = false
	if cs.stopChan != nil {
		close(cs.stopChan)
		cs.stopChan = nil
	}
}

func (cs *CronService) runLoop(stopChan chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			cs.checkJobs()
		}
	}
}

func (cs *CronService) checkJobs() {
	cs.mu.Lock()

	if !cs.running {
		cs.mu.Unlock()
		return
	}

	jobs, err := cs.store.List()
	if err != nil {
		cs.mu.Unlock()
		log.Printf("[cron] failed to list jobs: %v", err)
		return
	}

	now := time.Now().UnixMilli()
	var dueJobs []CronJob

	for i := range jobs {
		job := &jobs[i]
		if job.Enabled && job.State.NextRunAtMS != nil && *job.State.NextRunAtMS <= now {
			dueJobs = append(dueJobs, *job)
			// 清除 NextRunAtMS 避免重复执行
			job.State.NextRunAtMS = nil
			if saveErr := cs.store.Save(job); saveErr != nil {
				log.Printf("[cron] failed to save job state: %v", saveErr)
			}
		}
	}

	cs.mu.Unlock()

	// 在锁外执行任务
	for i := range dueJobs {
		cs.executeJob(&dueJobs[i])
	}
}

func (cs *CronService) executeJob(job *CronJob) {
	startTime := time.Now().UnixMilli()

	var err error
	if cs.onJob != nil {
		_, err = cs.onJob(job)
	}

	// 更新状态
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// 从 store 获取最新的 job
	current, getErr := cs.store.Get(job.ID)
	if getErr != nil {
		log.Printf("[cron] job %s disappeared before state update", job.ID)
		return
	}

	current.State.LastRunAtMS = &startTime
	current.UpdatedAtMS = time.Now().UnixMilli()

	if err != nil {
		current.State.LastStatus = "error"
		current.State.LastError = err.Error()
	} else {
		current.State.LastStatus = "ok"
		current.State.LastError = ""
	}

	// 计算下次运行时间
	if current.Schedule.Kind == "at" {
		if current.DeleteAfterRun {
			if delErr := cs.store.Delete(current.ID); delErr != nil {
				log.Printf("[cron] failed to delete one-time job: %v", delErr)
			}
			return
		}
		current.Enabled = false
		current.State.NextRunAtMS = nil
	} else {
		nextRun := cs.computeNextRun(&current.Schedule, time.Now().UnixMilli())
		current.State.NextRunAtMS = nextRun
	}

	if saveErr := cs.store.Save(current); saveErr != nil {
		log.Printf("[cron] failed to save job state: %v", saveErr)
	}
}

func (cs *CronService) computeNextRun(schedule *CronSchedule, nowMS int64) *int64 {
	switch schedule.Kind {
	case "at":
		if schedule.AtMS != nil && *schedule.AtMS > nowMS {
			return schedule.AtMS
		}
		return nil

	case "every":
		if schedule.EveryMS == nil || *schedule.EveryMS <= 0 {
			return nil
		}
		next := nowMS + *schedule.EveryMS
		return &next

	case "cron":
		if schedule.Expr == "" {
			return nil
		}
		now := time.UnixMilli(nowMS)
		nextTime, err := gronx.NextTickAfter(schedule.Expr, now, false)
		if err != nil {
			log.Printf("[cron] failed to compute next run for expr '%s': %v", schedule.Expr, err)
			return nil
		}
		nextMS := nextTime.UnixMilli()
		return &nextMS
	}

	return nil
}

func (cs *CronService) recomputeNextRuns() error {
	jobs, err := cs.store.List()
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	for i := range jobs {
		job := &jobs[i]
		if job.Enabled {
			job.State.NextRunAtMS = cs.computeNextRun(&job.Schedule, now)
			if saveErr := cs.store.Save(job); saveErr != nil {
				return saveErr
			}
		}
	}
	return nil
}

// SetOnJob 设置任务触发回调
func (cs *CronService) SetOnJob(handler JobHandler) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.onJob = handler
}

// AddJob 添加定时任务
func (cs *CronService) AddJob(name string, schedule CronSchedule, message string) (*CronJob, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	now := time.Now().UnixMilli()
	deleteAfterRun := (schedule.Kind == "at")

	job := &CronJob{
		ID:       generateID(),
		Name:     name,
		Enabled:  true,
		Schedule: schedule,
		Payload: CronPayload{
			Message: message,
		},
		State: CronJobState{
			NextRunAtMS: cs.computeNextRun(&schedule, now),
		},
		CreatedAtMS:    now,
		UpdatedAtMS:    now,
		DeleteAfterRun: deleteAfterRun,
	}

	if err := cs.store.Save(job); err != nil {
		return nil, err
	}

	return job, nil
}

// RemoveJob 删除任务
func (cs *CronService) RemoveJob(jobID string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

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

	job.Enabled = enabled
	job.UpdatedAtMS = time.Now().UnixMilli()

	if enabled {
		job.State.NextRunAtMS = cs.computeNextRun(&job.Schedule, time.Now().UnixMilli())
	} else {
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

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

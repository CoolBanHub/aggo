package cron

// Store 定义定时任务的存储接口
type Store interface {
	// List 列出所有任务
	List() ([]CronJob, error)

	// Get 根据 ID 获取任务
	Get(id string) (*CronJob, error)

	// Save 保存任务（新增或更新）
	Save(job *CronJob) error

	// Delete 根据 ID 删除任务
	Delete(id string) error
}

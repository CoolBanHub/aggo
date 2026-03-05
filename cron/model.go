package cron

// CronSchedule 定义任务的调度方式
type CronSchedule struct {
	Kind    string `json:"kind" gorm:"column:kind;type:varchar(10);not null"`   // "at" | "every" | "cron"
	AtMS    *int64 `json:"atMs,omitempty" gorm:"column:at_ms"`                  // Kind=at 时的触发时间戳（毫秒）
	EveryMS *int64 `json:"everyMs,omitempty" gorm:"column:every_ms"`            // Kind=every 时的间隔（毫秒）
	Expr    string `json:"expr,omitempty" gorm:"column:expr;type:varchar(100)"` // Kind=cron 时的 cron 表达式
}

// CronPayload 定义任务触发时携带的信息
type CronPayload struct {
	Message string `json:"message" gorm:"column:message;type:text"`
}

// CronJobState 定义任务的运行状态
type CronJobState struct {
	NextRunAtMS *int64 `json:"nextRunAtMs,omitempty" gorm:"column:next_run_at_ms"`
	LastRunAtMS *int64 `json:"lastRunAtMs,omitempty" gorm:"column:last_run_at_ms"`
	LastStatus  string `json:"lastStatus,omitempty" gorm:"column:last_status;type:varchar(20)"`
	LastError   string `json:"lastError,omitempty" gorm:"column:last_error;type:text"`
}

// CronJob 表示一个定时任务
type CronJob struct {
	ID             string       `json:"id" gorm:"column:id;primaryKey;type:varchar(32)"`
	Name           string       `json:"name" gorm:"column:name;type:varchar(100)"`
	Enabled        bool         `json:"enabled" gorm:"column:enabled"`
	Schedule       CronSchedule `json:"schedule" gorm:"embedded"`
	Payload        CronPayload  `json:"payload" gorm:"embedded"`
	State          CronJobState `json:"state" gorm:"embedded"`
	CreatedAtMS    int64        `json:"createdAtMs" gorm:"column:created_at_ms"`
	UpdatedAtMS    int64        `json:"updatedAtMs" gorm:"column:updated_at_ms"`
	DeleteAfterRun bool         `json:"deleteAfterRun" gorm:"column:delete_after_run"`
}

// TableName 指定 gorm 表名
func (CronJob) TableName() string {
	return "cron_jobs"
}

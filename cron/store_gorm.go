package cron

import (
	"fmt"

	"gorm.io/gorm"
)

// GormStore 基于 GORM 的数据库存储实现
// 支持 MySQL、PostgreSQL、SQLite
type GormStore struct {
	db *gorm.DB
}

// NewGormStore 创建 GORM 数据库存储
// 自动建表（如不存在），支持 MySQL、PostgreSQL、SQLite
func NewGormStore(db *gorm.DB) (Store, error) {
	if err := db.AutoMigrate(&CronJob{}); err != nil {
		return nil, fmt.Errorf("auto migrate cron_jobs failed: %w", err)
	}
	return &GormStore{db: db}, nil
}

// List 列出所有任务
func (s *GormStore) List() ([]CronJob, error) {
	var jobs []CronJob
	if err := s.db.Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("list jobs failed: %w", err)
	}
	return jobs, nil
}

// Get 根据 ID 获取任务
func (s *GormStore) Get(id string) (*CronJob, error) {
	var job CronJob
	if err := s.db.Where("id = ?", id).First(&job).Error; err != nil {
		return nil, fmt.Errorf("get job %s failed: %w", id, err)
	}
	return &job, nil
}

// Save 保存任务（新增或更新）
func (s *GormStore) Save(job *CronJob) error {
	if err := s.db.Save(job).Error; err != nil {
		return fmt.Errorf("save job failed: %w", err)
	}
	return nil
}

// Delete 删除任务
func (s *GormStore) Delete(id string) error {
	result := s.db.Where("id = ?", id).Delete(&CronJob{})
	if result.Error != nil {
		return fmt.Errorf("delete job %s failed: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("job %s not found", id)
	}
	return nil
}

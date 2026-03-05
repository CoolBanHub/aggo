package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// cronFileStore JSON 文件存储
type cronFileStore struct {
	version int
}

// fileStoreData JSON 文件存储结构
type fileStoreData struct {
	Version int       `json:"version"`
	Jobs    []CronJob `json:"jobs"`
}

type FileStore struct {
	path string
	data *fileStoreData
	mu   sync.RWMutex
}

// NewFileStore 创建文件存储
func NewFileStore(path string) Store {
	fs := &FileStore{
		path: path,
		data: &fileStoreData{
			Version: 1,
			Jobs:    []CronJob{},
		},
	}
	fs.load()
	return fs
}

func (s *FileStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, s.data)
}

func (s *FileStore) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 原子写入
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

// List 列出所有任务
func (s *FileStore) List() ([]CronJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]CronJob, len(s.data.Jobs))
	copy(result, s.data.Jobs)
	return result, nil
}

// Get 根据 ID 获取任务
func (s *FileStore) Get(id string) (*CronJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.data.Jobs {
		if s.data.Jobs[i].ID == id {
			job := s.data.Jobs[i]
			return &job, nil
		}
	}
	return nil, fmt.Errorf("job %s not found", id)
}

// Save 保存任务（新增或更新）
func (s *FileStore) Save(job *CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 查找是否存在，存在则更新
	for i := range s.data.Jobs {
		if s.data.Jobs[i].ID == job.ID {
			s.data.Jobs[i] = *job
			return s.save()
		}
	}

	// 不存在则新增
	s.data.Jobs = append(s.data.Jobs, *job)
	return s.save()
}

// Delete 删除任务
func (s *FileStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var jobs []CronJob
	found := false
	for _, job := range s.data.Jobs {
		if job.ID == id {
			found = true
			continue
		}
		jobs = append(jobs, job)
	}

	if !found {
		return fmt.Errorf("job %s not found", id)
	}

	s.data.Jobs = jobs
	return s.save()
}

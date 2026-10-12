package cron

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

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

// NewFileStore 创建文件存储。
//
// 兼容入口：加载失败（文件损坏、权限错误等）时只记录日志并返回空 store。
// 新代码请使用 NewFileStoreWithError，它会在加载失败时返回错误，
// 而不是把损坏文件当作空数据继续运行、并在下次 Save 时覆盖原文件。
//
// Deprecated: 使用 NewFileStoreWithError 以便观察到持久化错误。
func NewFileStore(path string) Store {
	fs, err := NewFileStoreWithError(path)
	if err != nil {
		log.Printf("[cron] file store load failed, continuing with empty store: %v", err)
		// 返回新的空 store（不能返回类型化的 nil 指针，否则接口非 nil 但调用会 panic）
		return &FileStore{
			path: path,
			data: &fileStoreData{
				Version: 1,
				Jobs:    []CronJob{},
			},
		}
	}
	return fs
}

// NewFileStoreWithError 创建文件存储，加载已有数据失败时返回错误。
// 文件不存在视为首次运行，返回空 store 与 nil 错误。
func NewFileStoreWithError(path string) (*FileStore, error) {
	fs := &FileStore{
		path: path,
		data: &fileStoreData{
			Version: 1,
			Jobs:    []CronJob{},
		},
	}
	if err := fs.load(); err != nil {
		return nil, fmt.Errorf("cron: load file store %s: %w", path, err)
	}
	return fs, nil
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

	// 原子写入：先写临时文件并 Sync，再 Rename，最后 Sync 目录，
	// 保证崩溃后要么是旧文件、要么是完整的新文件。
	tmpPath := s.path + ".tmp"
	if err := writeFileSync(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write temp file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("failed to replace store file %s: %w", s.path, err)
	}
	syncDir(dir)
	return nil
}

func writeFileSync(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// syncDir 对目录执行 Sync 以落盘 Rename 结果；失败只影响持久性保证，不视为错误。
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// List 列出所有任务
func (s *FileStore) List() ([]CronJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]CronJob, len(s.data.Jobs))
	for i := range s.data.Jobs {
		result[i] = *cloneJob(&s.data.Jobs[i])
	}
	return result, nil
}

// Get 根据 ID 获取任务
func (s *FileStore) Get(id string) (*CronJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.data.Jobs {
		if s.data.Jobs[i].ID == id {
			return cloneJob(&s.data.Jobs[i]), nil
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
			s.data.Jobs[i] = *cloneJob(job)
			return s.save()
		}
	}

	// 不存在则新增
	s.data.Jobs = append(s.data.Jobs, *cloneJob(job))
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

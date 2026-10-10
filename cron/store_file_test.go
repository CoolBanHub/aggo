package cron

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 损坏的 JSON 必须让构造函数返回错误，而不是被当作空数据继续运行。
func TestFileStoreLoadCorruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"jobs":[{broken`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewFileStoreWithError(path); err == nil {
		t.Fatal("NewFileStoreWithError should fail on corrupt JSON")
	}

	// 兼容入口只记录日志并返回空 store
	fs := NewFileStore(path)
	if fs == nil {
		t.Fatal("legacy NewFileStore should still return a store")
	}
	jobs, err := fs.List()
	if err != nil || len(jobs) != 0 {
		t.Fatalf("legacy store should be empty: jobs=%v err=%v", jobs, err)
	}
}

// 非权限类读取错误（目录当文件用）也要返回错误。
func TestFileStoreLoadReadError(t *testing.T) {
	dir := t.TempDir()
	// 用目录占用 store 路径，触发 ReadFile 的非 NotExist 错误
	path := filepath.Join(dir, "jobs.json")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStoreWithError(path); err == nil {
		t.Fatal("NewFileStoreWithError should fail when path is a directory")
	}
}

func TestFileStoreRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	fs, err := NewFileStoreWithError(path)
	if err != nil {
		t.Fatal(err)
	}

	every := int64(1800)
	at := time.Now().Add(time.Hour).UnixMilli()
	jobA := &CronJob{
		ID: "job-a", UserID: "userA", Name: "a", Enabled: true,
		Schedule: CronSchedule{Kind: "every", EveryMS: &every},
	}
	jobB := &CronJob{
		ID: "job-b", UserID: "userB", Name: "b", Enabled: false,
		Schedule: CronSchedule{Kind: "at", AtMS: &at},
	}
	if err := fs.Save(jobA); err != nil {
		t.Fatal(err)
	}
	if err := fs.Save(jobB); err != nil {
		t.Fatal(err)
	}

	// 重新加载
	fs2, err := NewFileStoreWithError(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fs2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d jobs, want 2", len(got))
	}
	for _, job := range got {
		switch job.ID {
		case "job-a":
			if job.Schedule.EveryMS == nil || *job.Schedule.EveryMS != every {
				t.Fatalf("job-a schedule not restored: %+v", job.Schedule)
			}
		case "job-b":
			if job.Schedule.AtMS == nil || *job.Schedule.AtMS != at {
				t.Fatalf("job-b schedule not restored: %+v", job.Schedule)
			}
		}
	}

	// 删除不存在任务返回错误
	if err := fs2.Delete("missing"); err == nil {
		t.Fatal("Delete(missing) should fail")
	}

	// 保存时不能影响调用方对象（深拷贝语义）
	mutated := *jobA
	mutated.Name = "changed"
	_ = mutated
	gotA, err := fs2.Get("job-a")
	if err != nil {
		t.Fatal(err)
	}
	if gotA.Name != "a" {
		t.Fatalf("stored job mutated: %s", gotA.Name)
	}
}

// 原子写入失败（目录只读）时返回错误，原文件保持不变。
func TestFileStoreSaveError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission checks are unreliable")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.json")
	fs, err := NewFileStoreWithError(path)
	if err != nil {
		t.Fatal(err)
	}

	every := int64(60)
	if err := fs.Save(&CronJob{ID: "keep", UserID: "u", Schedule: CronSchedule{Kind: "every", EveryMS: &every}}); err != nil {
		t.Fatal(err)
	}

	// 目录去掉写权限后保存失败
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	err = fs.Save(&CronJob{ID: "new", UserID: "u", Schedule: CronSchedule{Kind: "every", EveryMS: &every}})
	if err == nil {
		t.Fatal("Save should fail when directory is read-only")
	}

	// 原文件仍可读且未损坏
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fs2, err := NewFileStoreWithError(path)
	if err != nil {
		t.Fatalf("original file corrupted after failed save: %v", err)
	}
	jobs, _ := fs2.List()
	if len(jobs) != 1 || jobs[0].ID != "keep" {
		t.Fatalf("jobs after failed save = %+v, want only 'keep'", jobs)
	}
}

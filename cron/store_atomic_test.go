package cron

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreFailurePreservesCommittedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	store, err := NewFileStoreWithError(path)
	if err != nil {
		t.Fatal(err)
	}
	job := &CronJob{ID: "keep", Name: "original", Enabled: true}
	if err := store.Save(job); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	job.Name = "changed"
	if err := store.Save(job); err == nil {
		t.Fatal("expected update failure")
	}
	if err := store.Save(&CronJob{ID: "new"}); err == nil {
		t.Fatal("expected insert failure")
	}
	if err := store.Delete("keep"); err == nil {
		t.Fatal("expected delete failure")
	}
	got, err := store.Get("keep")
	if err != nil || got.Name != "original" {
		t.Fatalf("committed state changed: %+v %v", got, err)
	}
	if _, err := store.Get("new"); err == nil {
		t.Fatal("failed insert visible")
	}
	disk, err := NewFileStoreWithError(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err = disk.Get("keep")
	if err != nil || got.Name != "original" {
		t.Fatalf("disk state changed: %+v %v", got, err)
	}
}

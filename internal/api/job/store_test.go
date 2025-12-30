// internal/api/job/store_test.go
package job

import (
	"testing"
	"time"
)

func TestStore_CreateAndGet(t *testing.T) {
	store := NewStore(100, time.Hour)

	job := store.Create("backtest")
	if job.ID == "" {
		t.Error("expected job ID")
	}
	if job.Status != StatusPending {
		t.Errorf("expected pending, got %s", job.Status)
	}

	retrieved, err := store.Get(job.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.ID != job.ID {
		t.Error("IDs don't match")
	}
}

func TestStore_Update(t *testing.T) {
	store := NewStore(100, time.Hour)
	job := store.Create("backtest")

	err := store.Update(job.ID, func(j *Job) {
		j.Status = StatusRunning
		j.Progress = 50
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	retrieved, _ := store.Get(job.ID)
	if retrieved.Status != StatusRunning {
		t.Errorf("expected running, got %s", retrieved.Status)
	}
	if retrieved.Progress != 50 {
		t.Errorf("expected 50, got %d", retrieved.Progress)
	}
}

func TestStore_MaxSize(t *testing.T) {
	store := NewStore(2, time.Hour)

	job1 := store.Create("backtest")
	store.Create("backtest")
	store.Create("backtest") // Should evict job1

	_, err := store.Get(job1.ID)
	if err == nil {
		t.Error("expected job1 to be evicted")
	}
}

func TestStore_NotFound(t *testing.T) {
	store := NewStore(100, time.Hour)

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent job")
	}
}

func TestStore_List(t *testing.T) {
	store := NewStore(100, time.Hour)
	store.Create("backtest")
	store.Create("analysis")

	jobs := store.List()
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

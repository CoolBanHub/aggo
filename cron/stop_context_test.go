package cron

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStopDeadlineIncludesRealSchedulerShutdown(t *testing.T) {
	cs := NewCronService(newFakeStore(), nil)
	entered, release := make(chan struct{}, 1), make(chan struct{})
	cs.SetOnJobContext(func(ctx context.Context, job *CronJob) (string, error) {
		entered <- struct{}{}
		<-release // Deliberately ignores cancellation.
		return "ok", nil
	})
	interval := int64(10)
	if _, err := cs.AddJob("blocking", CronSchedule{Kind: "every", EveryMS: &interval}, "message", "u"); err != nil {
		t.Fatal(err)
	}
	if err := cs.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { close(release); cs.Stop() }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler never ran")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := cs.StopContext(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StopContext error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("deadline ignored: %v", elapsed)
	}
}

func TestStopCancelsScheduledJobContext(t *testing.T) {
	cs := NewCronService(newFakeStore(), nil)
	entered, canceled := make(chan struct{}, 1), make(chan struct{}, 1)
	// Even a builder returning an unrelated context cannot remove cancellation.
	cs.SetJobContextBuilder(func(context.Context, *CronJob) context.Context { return context.Background() })
	cs.SetOnJobContext(func(ctx context.Context, job *CronJob) (string, error) {
		entered <- struct{}{}
		<-ctx.Done()
		canceled <- struct{}{}
		return "", ctx.Err()
	})
	interval := int64(10)
	if _, err := cs.AddJob("cancel", CronSchedule{Kind: "every", EveryMS: &interval}, "message", "u"); err != nil {
		t.Fatal(err)
	}
	if err := cs.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler never ran")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := cs.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	default:
		t.Fatal("job context was not canceled")
	}
}

func TestInvalidScheduleNeverPersisted(t *testing.T) {
	store := newFakeStore()
	cs := NewCronService(store, nil)
	zero := int64(0)
	for _, schedule := range []CronSchedule{{Kind: "every", EveryMS: &zero}, {Kind: "cron", Expr: "invalid"}, {Kind: "at"}, {Kind: "unknown"}} {
		if _, err := cs.AddJob("bad", schedule, "msg", "u"); err == nil {
			t.Errorf("accepted invalid schedule: %+v", schedule)
		}
	}
	if store.count() != 0 {
		t.Fatal("invalid jobs were persisted")
	}
}

func TestFailedStartCleansScheduler(t *testing.T) {
	store := newFakeStore()
	store.listErr = errors.New("disk unavailable")
	cs := NewCronService(store, nil)
	if err := cs.Start(); err == nil {
		t.Fatal("expected Start error")
	}
	if cs.scheduler != nil || cs.running {
		t.Fatal("failed Start left scheduler running")
	}
	store.listErr = nil
	if err := cs.Start(); err != nil {
		t.Fatal(err)
	}
	cs.Stop()
}

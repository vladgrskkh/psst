package service_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/service"
)

type countingEngine struct {
	fullSyncs atomic.Int64
	pushes    atomic.Int64
	err       error
}

func (c *countingEngine) FullSync() error {
	c.fullSyncs.Add(1)
	return c.err
}
func (c *countingEngine) PushChanges() error {
	c.pushes.Add(1)
	return c.err
}
func (c *countingEngine) PullChanges() error { return c.err }

func TestSyncNowUpdatesStatus(t *testing.T) {
	t.Parallel()
	engine := &countingEngine{}
	svc := service.NewSyncService(engine, 10*time.Millisecond)
	if err := svc.SyncNow(); err != nil {
		t.Fatal(err)
	}
	if engine.fullSyncs.Load() != 1 {
		t.Fatalf("fullSyncs = %d; want 1", engine.fullSyncs.Load())
	}
	st := svc.Status()
	if st.Syncing || !st.LastSyncedAt.IsPresent() || st.LastError != nil {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestSyncNowRecordsError(t *testing.T) {
	t.Parallel()
	engine := &countingEngine{err: errors.New("boom")}
	svc := service.NewSyncService(engine, 10*time.Millisecond)
	if err := svc.SyncNow(); err == nil {
		t.Fatal("expected error")
	}
	if svc.Status().LastError == nil {
		t.Fatal("LastError must be recorded")
	}
}

func TestNotifyChangeDebounces(t *testing.T) {
	t.Parallel()
	engine := &countingEngine{}
	svc := service.NewSyncService(engine, 50*time.Millisecond)
	for range 10 {
		svc.NotifyChange()
	}
	time.Sleep(150 * time.Millisecond)
	if engine.pushes.Load() != 1 {
		t.Fatalf("pushes = %d; want 1 (debounced)", engine.pushes.Load())
	}
}

func TestPushNow(t *testing.T) {
	t.Parallel()
	engine := &countingEngine{}
	svc := service.NewSyncService(engine, 10*time.Millisecond)
	if err := svc.PushNow(); err != nil {
		t.Fatal(err)
	}
	if engine.pushes.Load() != 1 {
		t.Fatalf("pushes = %d; want 1", engine.pushes.Load())
	}
}

type sleepEngine struct {
	current atomic.Int64
	max     atomic.Int64
}

func (c *sleepEngine) enter() {
	n := c.current.Add(1)
	for {
		m := c.max.Load()
		if n <= m || c.max.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	c.current.Add(-1)
}

func (c *sleepEngine) FullSync() error    { c.enter(); return nil }
func (c *sleepEngine) PushChanges() error { c.enter(); return nil }
func (c *sleepEngine) PullChanges() error { return nil }

func TestSyncOperationsDoNotOverlap(t *testing.T) {
	t.Parallel()

	engine := &sleepEngine{}
	svc := service.NewSyncService(engine, 10*time.Millisecond)

	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		if err := svc.SyncNow(); err != nil {
			t.Errorf("SyncNow: %v", err)
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		if err := svc.PushNow(); err != nil {
			t.Errorf("PushNow: %v", err)
		}
	}()
	<-done
	<-done

	if got := engine.max.Load(); got != 1 {
		t.Fatalf("max concurrent engine calls = %d; want 1", got)
	}
}

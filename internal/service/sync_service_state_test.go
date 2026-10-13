//nolint:testpackage // coordinates the internal queue state while asserting the public Status value.
package service

import (
	"sync/atomic"
	"testing"
	"time"
)

type sequencedEngine struct {
	calls         atomic.Int32
	firstStarted  chan struct{}
	secondStarted chan struct{}
	releaseFirst  chan struct{}
	releaseSecond chan struct{}
}

func (e *sequencedEngine) FullSync() error {
	switch e.calls.Add(1) {
	case 1:
		close(e.firstStarted)
		<-e.releaseFirst
	case 2:
		close(e.secondStarted)
		<-e.releaseSecond
	}
	return nil
}

func (*sequencedEngine) PushChanges() error { return nil }
func (*sequencedEngine) PullChanges() error { return nil }

func TestSyncingStaysTrueWhileAnotherSyncIsQueued(t *testing.T) {
	t.Parallel()

	engine := &sequencedEngine{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
		releaseSecond: make(chan struct{}),
	}
	svc := NewSyncService(engine, time.Hour)
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		_ = svc.SyncNow()
		close(firstDone)
	}()
	<-engine.firstStarted
	go func() {
		_ = svc.SyncNow()
		close(secondDone)
	}()
	waitForActiveSyncs(t, svc, 2)
	close(engine.releaseFirst)
	<-firstDone
	<-engine.secondStarted
	if !svc.Status().Syncing {
		t.Fatal("Status().Syncing became false while the second sync was still running")
	}
	close(engine.releaseSecond)
	<-secondDone
	if svc.Status().Syncing {
		t.Fatal("Status().Syncing remained true after all syncs finished")
	}
}

func waitForActiveSyncs(t *testing.T, svc *SyncService, want int) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		svc.mu.Lock()
		active := svc.activeSyncs
		svc.mu.Unlock()
		if active == want {
			return
		}
		select {
		case <-timer.C:
			t.Fatalf("active syncs = %d; want %d", active, want)
		case <-ticker.C:
		}
	}
}

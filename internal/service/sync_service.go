package service

import (
	"sync"
	"time"

	"github.com/vladgrskkh/psst/pkg/option"
)

type SyncStatus struct {
	Syncing      bool
	LastSyncedAt option.Option[time.Time]
	LastError    error
}

type SyncService struct {
	engine   Syncer
	debounce time.Duration

	mu          sync.Mutex // Protects timer and status, which timer callbacks also update.
	status      SyncStatus
	timer       *time.Timer
	activeSyncs int

	engineMu sync.Mutex // Serializes timer callbacks with explicit sync operations.
}

func NewSyncService(engine Syncer, debounce time.Duration) *SyncService {
	return &SyncService{engine: engine, debounce: debounce}
}

// SyncNow runs a full sync synchronously.
func (s *SyncService) SyncNow() error {
	return s.runSync(s.engine.FullSync)
}

// PushNow runs a push synchronously.
func (s *SyncService) PushNow() error {
	return s.runSync(s.engine.PushChanges)
}

func (s *SyncService) runSync(syncOperation func() error) error {
	s.mu.Lock()
	s.activeSyncs++
	s.status.Syncing = true
	s.mu.Unlock()
	s.engineMu.Lock()
	err := syncOperation()
	s.engineMu.Unlock()
	s.mu.Lock()
	s.activeSyncs--
	s.status.Syncing = s.activeSyncs > 0
	s.status.LastError = err
	if err == nil {
		s.status.LastSyncedAt = option.Some(time.Now())
	}
	s.mu.Unlock()
	return err
}

// NotifyChange schedules a debounced push; rapid calls collapse into one.
func (s *SyncService) NotifyChange() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.debounce, func() {
		s.mu.Lock()
		s.activeSyncs++
		s.status.Syncing = true
		s.mu.Unlock()
		s.engineMu.Lock()
		err := s.engine.PushChanges()
		s.engineMu.Unlock()
		s.mu.Lock()
		s.activeSyncs--
		s.status.Syncing = s.activeSyncs > 0
		s.status.LastError = err
		if err == nil {
			s.status.LastSyncedAt = option.Some(time.Now())
		}
		s.mu.Unlock()
	})
}

func (s *SyncService) Status() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

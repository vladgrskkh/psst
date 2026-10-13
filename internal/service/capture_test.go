package service_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/internal/quickcapture"
	"github.com/vladgrskkh/psst/internal/service"
)

func TestCaptureKeepsLocalSuccessWhenPermissionOrSyncFails(t *testing.T) {
	t.Parallel()

	permissionErr := errors.New("permission check failed")
	syncErr := errors.New("sync failed")
	permissions := &capturePermissions{status: "authorized", err: permissionErr}
	syncer := &captureSyncer{err: syncErr}
	serviceUnderTest := service.NewCaptureService(
		service.NewTodoService(&captureStore{}, captureClock{}),
		permissions,
		syncer,
		"psst",
		slog.New(slog.DiscardHandler),
	)

	result, err := serviceUnderTest.Capture(&quickcapture.Result{Title: "buy milk"})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Title != "buy milk" {
		t.Fatalf("captured todo = %+v", result)
	}
	if permissions.calls != 1 || syncer.calls != 1 {
		t.Fatalf("permission calls = %d, sync calls = %d; want 1 each", permissions.calls, syncer.calls)
	}
}

func TestCaptureStopsBeforeRemoteWorkWhenCreateFails(t *testing.T) {
	t.Parallel()

	storeErr := errors.New("store failed")
	permissions := &capturePermissions{}
	syncer := &captureSyncer{}
	serviceUnderTest := service.NewCaptureService(
		service.NewTodoService(&captureStore{putErr: storeErr}, captureClock{}),
		permissions,
		syncer,
		"psst",
		slog.New(slog.DiscardHandler),
	)

	_, err := serviceUnderTest.Capture(&quickcapture.Result{Title: "buy milk"})
	if !errors.Is(err, storeErr) {
		t.Fatalf("Capture() error = %v; want store error", err)
	}
	if permissions.calls != 0 || syncer.calls != 0 {
		t.Fatalf("remote calls = permissions %d, sync %d; want 0", permissions.calls, syncer.calls)
	}
}

type captureStore struct {
	putErr error
}

func (s *captureStore) Get(string) (*domain.Todo, error) { return nil, service.ErrNotFound }
func (s *captureStore) List() ([]*domain.Todo, error)    { return nil, service.ErrNotFound }
func (s *captureStore) Put(*domain.Todo) error           { return s.putErr }

type captureClock struct{}

func (captureClock) Now() time.Time { return time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC) }

type capturePermissions struct {
	status string
	err    error
	calls  int
}

func (p *capturePermissions) Permissions() (string, error) {
	p.calls++
	return p.status, p.err
}

type captureSyncer struct {
	err   error
	calls int
}

func (s *captureSyncer) SyncNow() error {
	s.calls++
	return s.err
}

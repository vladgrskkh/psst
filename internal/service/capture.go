package service

import (
	"errors"
	"log/slog"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/internal/quickcapture"
)

// PermissionChecker reports the Reminders authorization state.
type PermissionChecker interface {
	Permissions() (string, error)
}

// SyncNower runs a full synchronization immediately.
type SyncNower interface {
	SyncNow() error
}

// CaptureService owns the quick-capture use case after parsing is complete.
type CaptureService struct {
	todos       *TodoService
	permissions PermissionChecker
	syncer      SyncNower
	defaultList string
	logger      *slog.Logger
}

func NewCaptureService(
	todos *TodoService,
	permissions PermissionChecker,
	syncer SyncNower,
	defaultList string,
	logger *slog.Logger,
) *CaptureService {
	return &CaptureService{
		todos:       todos,
		permissions: permissions,
		syncer:      syncer,
		defaultList: defaultList,
		logger:      logger,
	}
}

// Capture saves the parsed quick capture locally before attempting remote operations.
func (s *CaptureService) Capture(parsed *quickcapture.Result) (*domain.Todo, error) {
	listName := s.defaultList
	if parsed.List.IsPresent() {
		listName = parsed.List.ValueOr("")
	}
	params := CreateTodoParams{
		Title:       parsed.Title,
		Description: "",
		Deadline:    parsed.Deadline,
		ListName:    listName,
	}
	todo, err := s.todos.Create(params)
	if err != nil {
		return nil, err
	}
	status, permissionErr := s.permissions.Permissions()
	syncErr := s.syncer.SyncNow()
	if errors.Is(permissionErr, ErrRemindersAccessDenied) || status == "denied" {
		s.logger.Warn("reminders access denied")
	} else if permissionErr != nil {
		s.logger.Warn("permissions check failed", slog.Any("error", permissionErr))
	}
	if syncErr != nil {
		s.logger.Warn("initial sync failed", slog.Any("error", syncErr))
	}
	return todo, nil
}

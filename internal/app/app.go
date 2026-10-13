// Package app assembles the application services and owns their infrastructure resources.
package app

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/vladgrskkh/psst/internal/config"
	"github.com/vladgrskkh/psst/internal/infra/reminders"
	"github.com/vladgrskkh/psst/internal/infra/store"
	"github.com/vladgrskkh/psst/internal/service"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// LoadConfig reads PSST_CONFIG or the default user configuration path.
func LoadConfig() (*config.Config, error) {
	path := os.Getenv("PSST_CONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}
		path = filepath.Join(home, ".config", "psst", "config.toml")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("config error: %w", err)
	}
	return cfg, nil
}

// App holds the wired services and resources for one CLI or TUI session.
type App struct {
	// CaptureService is the wired quick-capture use case.
	CaptureService *service.CaptureService
	// TodoService reads and changes the locally stored todos.
	TodoService *service.TodoService
	logger      *slog.Logger
	logFile     *os.File
	db          io.Closer
}

// New opens persistence and constructs the application services for cfg.
func New(cfg *config.Config) (*App, error) {
	logger, logFile := newLogger(cfg)
	for _, warning := range cfg.Warnings {
		logger.Warn("config warning", slog.String("warning", warning))
	}

	dbPath := cfg.General.DBPath
	if dbPath == "" {
		var err error
		dbPath, err = config.DefaultDBPath()
		if err != nil {
			closeLog(logFile)
			return nil, err
		}
	}
	db, err := store.Open(dbPath)
	if err != nil {
		closeLog(logFile)
		return nil, err
	}

	todoStore := store.NewTodoStore(db)
	clock := systemClock{}
	todos := service.NewTodoService(todoStore, clock)
	reminderClient := reminders.NewClient(logger)
	engine := service.NewSyncEngine(
		todoStore,
		todoStore,
		reminderClient,
		clock,
		cfg.General.DefaultList,
		time.Duration(cfg.General.TombstoneRetentionDays)*24*time.Hour,
	)
	syncer := service.NewSyncService(engine, time.Duration(cfg.General.PushDebounceMs)*time.Millisecond)
	return &App{
		TodoService: todos,
		CaptureService: service.NewCaptureService(
			todos,
			permissionChecker{client: reminderClient},
			syncer,
			cfg.General.DefaultList,
			logger,
		),
		logger:  logger,
		logFile: logFile,
		db:      db,
	}, nil
}

func newLogger(cfg *config.Config) (*slog.Logger, *os.File) {
	logPath, err := config.DefaultLogPath()
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), nil
	}
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), nil
	}
	level := slog.LevelInfo
	if parseErr := level.UnmarshalText([]byte(cfg.General.LogLevel)); parseErr != nil {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})), f
}

type permissionChecker struct {
	client *reminders.Client
}

func (p permissionChecker) Permissions() (string, error) {
	status, err := p.client.Permissions()
	if errors.Is(err, reminders.ErrAccessDenied) {
		return status, service.ErrRemindersAccessDenied
	}
	return status, err
}

// Close releases the database and log file.
func (c *App) Close() error {
	var errs []error
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			wrappedErr := fmt.Errorf("close database: %w", err)
			errs = append(errs, wrappedErr)
			c.logger.Warn("close database", slog.Any("error", err))
		}
	}
	if c.logFile != nil {
		if err := c.logFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close log: %w", err))
		}
	}
	return errors.Join(errs...)
}

func closeLog(file *os.File) {
	if file != nil {
		_ = file.Close()
	}
}

package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladgrskkh/psst/internal/app"
	"github.com/vladgrskkh/psst/internal/config"
	"github.com/vladgrskkh/psst/internal/service"
)

func TestLoadConfigReturnsMalformedFileError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.toml")
	if err := os.WriteFile(path, []byte("[unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PSST_CONFIG", path)

	cfg, err := app.LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() succeeded with malformed TOML")
	}
	if cfg != nil {
		t.Fatalf("LoadConfig() config = %+v; want nil on error", cfg)
	}
}

func TestTodoServiceUsesApplicationPersistence(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.General.DBPath = filepath.Join(t.TempDir(), "psst.db")
	var application *app.App
	{
		var err error
		application, err = app.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Error(err)
		}
	})
	todo, err := application.TodoService.Create(service.CreateTodoParams{
		Title:    "Read this in the dashboard",
		ListName: "psst",
	})
	if err != nil {
		t.Fatal(err)
	}
	todos, err := application.TodoService.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 1 || todos[0].ID != todo.ID || todos[0].Title != "Read this in the dashboard" {
		t.Fatalf("wired todo service returned %+v", todos)
	}
}

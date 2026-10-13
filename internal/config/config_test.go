package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vladgrskkh/psst/internal/config"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	if cfg.General.DefaultList != "psst" {
		t.Fatalf("DefaultList = %q; want psst", cfg.General.DefaultList)
	}
	if cfg.General.SyncIntervalSeconds != 30 || cfg.General.PushDebounceMs != 500 ||
		cfg.General.TombstoneRetentionDays != 30 || cfg.General.DefaultTime != "09:00" {
		t.Fatalf("unexpected general defaults: %+v", cfg.General)
	}
	if cfg.Theme.Bg != "#1e1e2e" || cfg.Theme.Accent != "#89b4fa" || cfg.Theme.Overdue != "#f38ba8" {
		t.Fatalf("unexpected theme defaults: %+v", cfg.Theme)
	}
	if !reflect.DeepEqual(cfg.Keys.Up, []string{"k", "up"}) ||
		!reflect.DeepEqual(cfg.Keys.Quit, []string{"q", "ctrl+c"}) {
		t.Fatalf("unexpected key defaults: %+v", cfg.Keys)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.General.DefaultList != "psst" {
		t.Fatalf("DefaultList = %q; want psst", cfg.General.DefaultList)
	}
}

func TestLoadPartialOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[general]
default_list = "groceries"
sync_interval_seconds = 60

[theme]
accent = "#ff0000"

[keys]
up = ["w"]

[unknown]
foo = "bar"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.General.DefaultList != "groceries" {
		t.Fatalf("DefaultList = %q; want groceries", cfg.General.DefaultList)
	}
	if cfg.General.SyncIntervalSeconds != 60 {
		t.Fatalf("SyncIntervalSeconds = %d; want 60", cfg.General.SyncIntervalSeconds)
	}
	if cfg.Theme.Accent != "#ff0000" || cfg.Theme.Bg != "#1e1e2e" {
		t.Fatalf("theme override failed: %+v", cfg.Theme)
	}
	if !reflect.DeepEqual(cfg.Keys.Up, []string{"w"}) {
		t.Fatalf("Keys.Up = %v; want [w]", cfg.Keys.Up)
	}
	if len(cfg.Warnings) == 0 {
		t.Fatal("expected warnings for unknown keys")
	}
}

func TestStateDir(t *testing.T) {
	t.Parallel()

	dir, err := config.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "state", "psst")
	if dir != want {
		t.Fatalf("StateDir = %q; want %q", dir, want)
	}
}

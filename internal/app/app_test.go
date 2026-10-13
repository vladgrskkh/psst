package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladgrskkh/psst/internal/app"
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

//nolint:testpackage // tests the private command factory.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/vladgrskkh/psst/internal/quickcapture"
)

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), version) {
		t.Fatalf("version output = %q", stdout.String())
	}
}

//nolint:paralleltest // configureCLI changes the configuration environment.
func TestNoArgsOpensDashboard(t *testing.T) {
	dbPath := configureCLI(t)
	cmd := newRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetIn(strings.NewReader("q"))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("dashboard did not open local persistence: %v", err)
	}
	if strings.Contains(stdout.String(), "plan 2") || !strings.Contains(stdout.String(), "psst") {
		t.Fatalf("dashboard output = %q", stdout.String())
	}
	requireClosedDatabase(t, dbPath)
}

//nolint:paralleltest // configureCLI changes the configuration environment.
func TestNoArgsReturnsConfigError(t *testing.T) {
	dbPath := configureCLI(t)
	path := filepath.Join(filepath.Dir(dbPath), "config.toml")
	if err := os.WriteFile(path, []byte("[unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("dashboard opened with invalid configuration")
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid config opened persistence: %v", err)
	}
}

//nolint:paralleltest // configureCLI changes the configuration environment.
func TestNoArgsReportsDatabaseOpenFailure(t *testing.T) {
	dbPath := configureCLI(t)
	dir := filepath.Dir(dbPath)
	configPath := filepath.Join(dir, "config.toml")
	content := fmt.Sprintf("[general]\ndb_path = %q\n", dir)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{}, &stdout, &stderr); code != runtimeErrorExitCode {
		t.Fatalf("database-open failure exit code = %d; want %d", code, runtimeErrorExitCode)
	}
	if !strings.Contains(stderr.String(), "open bbolt db") {
		t.Fatalf("startup error did not report the database failure: %q", stderr.String())
	}
}

//nolint:paralleltest // configureCLI changes the configuration environment.
func TestNoArgsClosesDatabaseAfterCancellation(t *testing.T) {
	dbPath := configureCLI(t)
	cmd := newRootCommand()
	cmd.SetArgs([]string{})
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&bytes.Buffer{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := cmd.ExecuteContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("dashboard cancellation = %v; want context.Canceled", err)
	}
	requireClosedDatabase(t, dbPath)
}

func TestExecuteUnknownCommandReturnsUsageExitCode(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"wat"}, &stdout, &stderr); code != usageExitCode {
		t.Fatalf(
			"Execute() exit code = %d; want %d (stdout %q, stderr %q)",
			code,
			usageExitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestExecuteUnknownFlagReturnsUsageExitCode(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"--bad"}, &stdout, &stderr); code != usageExitCode {
		t.Fatalf(
			"Execute() exit code = %d; want %d (stdout %q, stderr %q)",
			code,
			usageExitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestNewRequiresExactlyOneCaptureArgument(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	cmd.SetArgs([]string{"new", "buy milk", "ignored"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("new accepted more than one capture argument")
	}
}

//nolint:paralleltest // configureCLI calls t.Setenv, which cannot run in parallel.
func TestNewCreatesTodoAndRunsOneFullSync(t *testing.T) {
	configureCLI(t)
	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"new", "buy milk @tomorrow #groceries"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `created: "buy milk"`) ||
		!strings.Contains(stdout.String(), "list groceries") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

//nolint:paralleltest // configureCLI calls t.Setenv, which cannot run in parallel.
func TestNewKeepsLocalSuccessWhenSyncFails(t *testing.T) {
	configureCLI(t)
	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"new", "buy milk"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v; local capture should succeed", err)
	}
	if !strings.Contains(stdout.String(), `created: "buy milk"`) || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

//nolint:paralleltest // configureCLI calls t.Setenv, which cannot run in parallel.
func TestInvalidCaptureDoesNotCreateApplication(t *testing.T) {
	dbPath := configureCLI(t)
	cmd := newRootCommand()
	cmd.SetArgs([]string{"new", "@tomorrow"})
	if err := cmd.Execute(); !errors.Is(err, quickcapture.ErrEmptyTitle) {
		t.Fatalf("Execute() error = %v; want ErrEmptyTitle", err)
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database exists after invalid capture: stat error = %v", err)
	}
}

func configureCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "psst.db")
	configPath := filepath.Join(dir, "config.toml")
	content := fmt.Sprintf("[general]\ndb_path = %q\n", dbPath)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PSST_CONFIG", configPath)
	return dbPath
}

func requireClosedDatabase(t *testing.T, path string) {
	t.Helper()
	var db *bolt.DB
	{
		var err error
		db, err = bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
		if err != nil {
			t.Fatalf("dashboard retained database lock: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

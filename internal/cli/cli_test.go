//nolint:testpackage // tests the private command factory.
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestNoArgsPrintsPlanNote(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "plan 2") {
		t.Fatalf("output = %q", stdout.String())
	}
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

func TestNewCreatesTodoAndRunsOneFullSync(t *testing.T) {
	t.Parallel()

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

func TestNewKeepsLocalSuccessWhenSyncFails(t *testing.T) {
	t.Parallel()

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

func TestInvalidCaptureDoesNotCreateApplication(t *testing.T) {
	t.Parallel()

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

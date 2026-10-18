//nolint:testpackage // tests the private command factory.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
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

//nolint:paralleltest,reassign // configureCLI changes environment and the test drives a real terminal stream.
func TestNoArgsCreateEditAndCancelInTemporaryDatabase(t *testing.T) {
	dbPath := configureCLI(t)
	input, send, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close(); _ = send.Close() })
	previousStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = previousStdin })
	output := &formJourneyOutput{input: send}
	deadline := time.AfterFunc(5*time.Second, func() { _, _ = send.WriteString("\x03") })
	defer deadline.Stop()
	code := Execute([]string{}, output, &bytes.Buffer{})
	if !output.created || !output.edited || !output.canceled || !output.quit || code != 0 {
		t.Fatalf(
			"form interaction created=%v edited=%v canceled=%v quit=%v code=%d output=%q",
			output.created,
			output.edited,
			output.canceled,
			output.quit,
			code,
			ansi.Strip(output.buffer.String()),
		)
	}
	requireClosedDatabase(t, dbPath)
}

type formJourneyOutput struct {
	mu                              sync.Mutex
	buffer                          bytes.Buffer
	input                           *os.File
	stage                           int
	created, edited, canceled, quit bool
}

func (o *formJourneyOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n, err := o.buffer.Write(data)
	if err != nil {
		return n, err
	}
	frame := ansi.Strip(string(data))
	switch o.stage {
	case 0:
		if strings.Contains(frame, "psst") {
			o.stage = 1
			_, err = o.input.WriteString("n")
		}
	case 1:
		if strings.Contains(frame, "New todo") {
			o.stage = 2
			o.after(3, "Terminal task\t\t\t\r")
		}
	case 3:
		if strings.Contains(frame, "> [ ] Terminal task") {
			o.created = true
			o.stage = 4
			_, err = o.input.WriteString("e")
			o.after(5, "\x01\x0bUpdated task\r")
		}
	case 5:
		if strings.Contains(frame, "> [ ] Updated task") {
			o.edited = true
			o.stage = 6
			_, err = o.input.WriteString("e")
			o.after(7, "\x1b")
		}
	}
	return n, err
}

func (o *formJourneyOutput) after(next int, keys string) {
	time.AfterFunc(150*time.Millisecond, func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if o.stage != next-1 {
			return
		}
		o.stage = next
		if _, err := o.input.WriteString(keys); err != nil {
			return
		}
		if keys == "\x1b" {
			o.canceled = true
			time.AfterFunc(150*time.Millisecond, func() {
				o.mu.Lock()
				defer o.mu.Unlock()
				o.stage = 9
				o.quit = true
				_, _ = o.input.WriteString("q")
			})
		}
	})
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

//nolint:paralleltest,reassign // real Execute uses stdin; this serial test restores it via cleanup.
func TestExecuteReportsDashboardMutationFailureAfterTerminalExit(t *testing.T) {
	dbPath := configureCLI(t)
	seedMismatchedTodo(t, dbPath)
	input, send, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close(); _ = send.Close() })
	previousStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = previousStdin })
	output := &dashboardActionOutput{input: send}
	var stderr bytes.Buffer
	// A deadline only bounds a broken interaction; rendered output drives input.
	deadline := time.AfterFunc(5*time.Second, func() { _, _ = send.WriteString("\x03") })
	defer deadline.Stop()
	code := Execute([]string{}, output, io.MultiWriter(&stderr, output))
	if !output.toggled || !output.quit {
		t.Fatalf("dashboard interaction did not reach the visible error: %q", output.String())
	}
	if code != runtimeErrorExitCode {
		t.Errorf("failed dashboard mutation exit code = %d; want %d", code, runtimeErrorExitCode)
	}
	want := "psst: Could not toggle todo: todo not found"
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("mutation failure missing from stderr: %q", stderr.String())
	}
	text := output.String()
	exit := strings.LastIndex(text, "\x1b[?1049l")
	if exit < 0 || !strings.Contains(text[exit:], want) {
		t.Errorf("mutation failure missing after alternate-screen exit: %q", text)
	}
	requireClosedDatabase(t, dbPath)
}

type dashboardActionOutput struct {
	bytes.Buffer

	input         *os.File
	toggled, quit bool
}

func (o *dashboardActionOutput) Write(data []byte) (int, error) {
	n, err := o.Buffer.Write(data)
	if err != nil {
		return n, err
	}
	view := ansi.Strip(o.String())
	var key string
	switch {
	case !o.toggled && strings.Contains(view, "> [ ] broken todo"):
		o.toggled, key = true, "x"
	case !o.quit && strings.Contains(view, "Could not toggle todo: todo not found"):
		o.quit, key = true, "q"
	}
	if key != "" {
		_, err = o.input.WriteString(key)
	}
	return n, err
}

func seedMismatchedTodo(t *testing.T, path string) {
	t.Helper()
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, createErr := tx.CreateBucketIfNotExists([]byte("todos"))
		if createErr != nil {
			return createErr
		}
		return bucket.Put(
			[]byte("stored-key"),
			[]byte(`{"id":"missing-key","title":"broken todo"}`),
		)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
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

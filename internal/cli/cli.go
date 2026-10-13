// Package cli defines the psst command-line interface.
package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vladgrskkh/psst/internal/app"
	"github.com/vladgrskkh/psst/internal/quickcapture"
)

const (
	version              = "0.1.0-dev"
	usageExitCode        = 2
	runtimeErrorExitCode = 1
)

var errUsage = errors.New("usage: psst new \"<text>\" | psst --version")

// Execute runs psst with args and returns the process exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	cmd := newRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(stderr, "psst:", err)
		_, _, findErr := cmd.Find(args)
		if errors.Is(err, errUsage) || findErr != nil {
			return usageExitCode
		}
		return runtimeErrorExitCode
	}
	return 0
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "psst",
		Short:         "Capture todos and sync them with macOS Reminders",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(
				cmd.OutOrStdout(),
				"psst: the TUI lands in plan 2 — try `psst new \"buy milk @tomorrow 9:00\"`",
			)
			return err
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %w", errUsage, err)
	})
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	root.AddCommand(&cobra.Command{
		Use:   "new <text>",
		Short: "Create a todo from a quick-capture string",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errUsage
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNewCommand(args[0], cmd.OutOrStdout())
		},
	})
	return root
}

func runNewCommand(text string, stdout io.Writer) error {
	cfg, err := app.LoadConfig()
	if err != nil {
		return err
	}
	parsed, err := quickcapture.Parse(text, time.Now(), cfg.General.DefaultTime)
	if err != nil {
		return err
	}
	application, err := app.New(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = application.Close() }()
	todo, err := application.CaptureService.Capture(parsed)
	if err != nil {
		return err
	}

	var out strings.Builder
	fmt.Fprintf(&out, "✓ created: %q", todo.Title)
	if todo.Deadline.IsPresent() {
		out.WriteString(" — due ")
		out.WriteString(todo.Deadline.ValueOr(time.Time{}).Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(&out, " — list %s\n", todo.ListName)
	_, err = io.WriteString(stdout, out.String())
	return err
}

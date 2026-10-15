# psst TUI Dashboard Implementation Plan (Part 2)

> **For agentic workers:** Execute the tasks against the approved spec with bounded implementation ownership and independent review. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Opening `psst` displays a read-only, keyboard-navigable dashboard of locally stored todos.

**Architecture:** `internal/ui` owns the Bubble Tea model and presentation, depending on a narrow todo-listing interface. `internal/app` exposes its existing todo service; the Cobra root command owns terminal startup and closes the application after the program exits. UI service calls run in `tea.Cmd`, never directly in `Update`.

**Tech Stack:** Go 1.25.4, Bubble Tea v1.3.10, Bubbles v0.21.0, Lip Gloss v1.1.0, existing bbolt and configuration packages; stdlib tests.

**Spec:** `docs/superpowers/specs/2026-08-16-psst-core-tui-design.md` (approved).

**Status:** Slice 1 is merged in main at `7291aa4`. Core and CI are already merged. Slice 2 dashboard filtering on `tui-filter` is implemented, audited and independently reviewed. Toggle/delete remain the next pending dashboard interactions. Relative-date capture is a separate PR.

## Global Constraints

- Keep each PR focused on one independently reviewable outcome.
- `internal/app` remains the composition root. UI and CLI do not import infra.
- Consumers depend on narrow interfaces. This dashboard only needs `List(string) ([]*domain.Todo, error)`.
- Use `option.Option[T]` for optional domain values; do not introduce pointer fields for optionality.
- Build styles from `config.ThemeConfig` once; do not hardcode colors.
- Honor configured navigation/quit keys; defaults are up `[k, up]`, down `[j, down]`, quit `[q, ctrl+c]`.
- Keep `cmd/psst/main.go` as the thin process entry point. Keep quick capture and `--version` working.
- `make audit` must pass before publication. Add tests for user-visible behavior, using stdlib `testing`.

## Review Focus

- Tiny/narrow terminals and long or multiline titles: rendering stays within the window and the selected row stays visible.
- Calendar boundaries and time zones: classify deadlines using the launch reference time's location; tomorrow is a calendar day, not 24 hours.
- Empty/all-completed data and reader errors: dashboard renders useful feedback and remains quittable.
- Custom bindings: configured navigation/quit works and superseded default keys are not advertised as active.
- Program failure or immediate quit: close the database and log; CLI reports runtime errors without opening the Reminders bridge.

## Task 1: Read-only dashboard

**Files:** create `internal/ui/dashboard.go`, `internal/ui/dashboard_view.go`, `internal/ui/theme.go`, `internal/ui/dashboard_test.go`; modify `go.mod`, `go.sum`.

**Interfaces:** consumes domain todos and config theme/keys. Produces `TodoLister` (`List(string) ([]*domain.Todo, error)`) and `NewDashboard(reader TodoLister, theme config.ThemeConfig, keys config.KeysConfig, now time.Time) Dashboard`, where `Dashboard` implements `tea.Model`.

- [x] Write failing tests for initialization through a returned command, load-error/empty feedback, calendar grouping, navigation/remaps/quit, window bounds and selected-row visibility. Use an injected reader only for the I/O boundary; assert real model/view behavior.
- [x] Run `go test ./internal/ui` and record the expected missing-feature failure before implementing.
- [x] Implement the model and rendering. Render OVERDUE, TODAY, TOMORROW, LATER and a collapsed DONE count. Completed/deleted todos are not selectable. Past instants are overdue; remaining deadlines today/tomorrow use local calendar dates; absent/later deadlines go to LATER. Sort deterministically within groups without mutating the source snapshot. Use one line per row, cursor highlight, and a footer advertising only implemented bindings. Initial loading and errors are visible.
- [x] Keep `Update` pure, return the list-loading command from `Init`, and keep all presentation colors in `theme.go` sourced from config. Handle window resize without negative dimensions or losing the selected row. Do not add editing, mutation, filtering, sync controls, view switching, progress calculations, or speculative abstractions in this slice.
- [x] Run focused UI tests. Report evidence and files changed for integration/review.

## Task 2: Launch the dashboard

**Files:** modify `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/cli.go`, `internal/cli/cli_test.go`.

**Interfaces:** consume Task 1's constructor. Expose `App.TodoService *service.TodoService`; `CaptureService` uses the same service instance.

- [x] Replace the placeholder root-command test with a real-program test using configured temporary persistence and Cobra input/output streams. Verify no-argument invocation enters/exits the TUI, releases the DB lock, and config/startup errors propagate. Preserve version, unknown-command, and quick-capture coverage.
- [x] Run the focused test and observe failure while the old placeholder is still present.
- [x] Expose the todo service during existing application assembly. Replace the root placeholder directly with config/app loading and `tea.NewProgram`, passing the command's context/input/output and alternate-screen option. Return program and cleanup errors via `errors.Join`. No new pass-through runner wrappers or test-only production hooks.
- [x] Run `make audit` and `make build/psst`; verify the dashboard in a temporary database/terminal and perform independent review of both tasks.

Publication: commit and open a focused PR describing the final read-only dashboard and its validation. Independent review's space-binding and final-row scrolling findings were fixed with RED-to-GREEN regressions; the final integrated audit and scoped re-review pass.

## Following slices from the same approved spec

### Slice 2: Dashboard filtering

**Files:** modify `internal/ui/dashboard.go`, `internal/ui/dashboard_view.go`, `internal/ui/dashboard_test.go`, `go.mod`, `go.sum`, and this plan. Reuse the existing theme styles.

- [x] Add behavior tests and observe meaningful RED for filter submission, draft text, cancellation without I/O, blank clearing, Unicode editing/paste, remapped keys, loading/error/no-match feedback, terminal bounds and latest-response protection.
- [x] Implement configured filter/cancel bindings with a Bubbles v0.21.0 text input compatible with Bubble Tea v1. Keep `NewDashboard` and the one-method `TodoLister` unchanged. Preserve applied rows/selection while editing; Enter returns a deferred `List(query)` command. Track request generations so initial/older results and errors cannot replace a newer search.
- [x] Verify focused UI tests, full `make audit`, build and independent review. Exercise search, draft cancellation and configured quit in a real terminal. Confirm active help follows Quit/Cancel/submit precedence while editing.

### Remaining slices

1. Dashboard toggle/delete interactions through todo-service commands, with confirmation and reload messages.
2. Shared new/edit form: title, natural-language deadline validation, description, cached-list picker, return to previous view.
3. Minimal-list view and root screen routing; configurable view switching.
4. Sync lifecycle/status, launch/full/manual sync, periodic pull, mutation-triggered debounce, shutdown coordination and refresh messages.
5. Weekly progress, completed-section expansion and remaining visual polish against the approved theme.

These are separate reviewable PRs. Slice 2 contains filtering only; toggle/delete, forms and the other interactions remain pending.

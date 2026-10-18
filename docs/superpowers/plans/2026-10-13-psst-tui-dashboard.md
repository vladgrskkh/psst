# psst TUI Dashboard Implementation Plan (Part 2)

> **For agentic workers:** Execute the tasks against the approved spec with bounded implementation ownership and independent review. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Opening `psst` displays a keyboard-navigable dashboard of locally stored todos, delivered in small interaction slices.

**Architecture:** `internal/ui` owns the Bubble Tea model and presentation, depending on a narrow interface for listing and the implemented dashboard actions. `internal/app` exposes its existing todo service; the Cobra root command owns terminal startup and closes the application after the program exits. UI service calls run in `tea.Cmd`, never directly in `Update`.

**Tech Stack:** Go 1.25.4, Bubble Tea v1.3.10, Bubbles v0.21.0, Lip Gloss v1.1.0, existing bbolt and configuration packages; stdlib tests.

**Spec:** `docs/superpowers/specs/2026-08-16-psst-core-tui-design.md` (approved).

**Status:** Slice 1 is merged at `7291aa4`, relative-date capture at `5512a32`, Slice 2 filtering at `6b0e880`, and dashboard actions at `23b13a9`. Task 4 implements the shared create/edit form and Dashboard/Form routing on `tui-form`; audit/build and independent review pass, including fixes for typed list names and printable quit keys. Later slices remain separate PRs.

## Global Constraints

- Keep each PR focused on one independently reviewable outcome.
- `internal/app` remains the composition root. UI and CLI do not import infra.
- Consumers depend on narrow interfaces. Dashboard actions need only `List(string) ([]*domain.Todo, error)`, `ToggleDone(string) (*domain.Todo, error)`, and `Delete(string) error` from the existing todo service.
- Use `option.Option[T]` for optional domain values; do not introduce pointer fields for optionality.
- Build styles from `config.ThemeConfig` once; do not hardcode colors.
- Honor configured navigation/quit keys; defaults are up `[k, up]`, down `[j, down]`, quit `[q, ctrl+c]`.
- Honor configured action bindings; defaults are toggle `[space, x]`, delete `[d]`, confirm `[y]`, cancel `[esc]` with `n` also cancelling the delete prompt.
- Keep `cmd/psst/main.go` as the thin process entry point. Keep quick capture and `--version` working.
- `make audit` must pass before publication. Add tests for user-visible behavior, using stdlib `testing`.

## Review Focus

- Tiny/narrow terminals and long or multiline titles: rendering stays within the window and the selected row stays visible.
- Calendar boundaries and time zones: classify deadlines using the launch reference time's location; tomorrow is a calendar day, not 24 hours.
- Empty/all-completed data and reader errors: dashboard renders useful feedback and remains quittable.
- Custom bindings: configured navigation/quit works and superseded default keys are not advertised as active.
- Program failure or immediate quit: close the database and log; CLI reports runtime errors without opening the Reminders bridge.
- Mutation shutdown ordering: accepted writes finish before intentional quit, and a requested quit cannot admit another write before Bubble Tea processes its quit message.

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

### Slice 3: Dashboard toggle/delete

## Task 3: Local dashboard actions

**Files:** modify `internal/ui/dashboard.go`, `internal/ui/dashboard_view.go`, `internal/ui/dashboard_test.go`; create `internal/ui/dashboard_actions.go` and `internal/ui/dashboard_actions_test.go`. The final review adds focused `internal/cli/cli.go` and `internal/cli/cli_test.go` changes to report failed writes after terminal restoration. Keep behavior-specific helpers in the actions file; do not refactor unrelated UI or services. This plan is maintained by the coordinating agent.

**Interfaces:** replace the dashboard's listing-only consumer with `DashboardTodos`, containing exactly `List(string) ([]*domain.Todo, error)`, `ToggleDone(string) (*domain.Todo, error)`, and `Delete(string) error`. Change the existing `NewDashboard` parameter to this interface; `App.TodoService` already satisfies it, so CLI composition needs no adapter. Retain the existing constructor's other arguments and `tea.Model` contract.

- [x] Write behavior tests first and record meaningful RED before implementation. Use the real todo service with an in-memory store for successful mutation/reload flows; a narrow I/O double may capture deferred calls and inject failures. Keep tests in package `ui_test` and use existing rendering/key helpers where applicable.
- [x] Configured `ToggleDone` (default actual space event or `x`) schedules `ToggleDone(selectedID)` only when a selectable todo is present. Store the stable ID on presentation rows; never infer identity from a title or cursor index. Selection-less loading/error/empty/all-completed states return no mutation command. Update/constructor/View must not perform I/O.
- [x] Configured `Delete` (default `d`) shows an inline confirmation naming the selected todo without I/O. Pin its ID/title until the prompt is resolved; navigation/filter/toggle/delete keys cannot retarget it. Configured `Confirm` (default `y`) schedules `Delete(pinnedID)`; `n` or configured `Cancel` (default Esc) dismisses it without I/O. Quit remains available. In a prompt, precedence is Quit, Cancel/`n`, Confirm; advertise only effective bindings.
- [x] While a mutation or its follow-up reload is pending, prevent repeated mutations and modal/filter/navigation changes, but honor quit and resize. Show a pending status. On successful mutation, return a separate reload command using the applied filter; rebuild the rows from that result, updating group/DONE counts without optimistic domain mutations. It is sufficient to reset selection to the first remaining open todo, consistent with existing filtering.
- [x] A quit request during a pending write is remembered and fulfilled once the service call completes, without a redundant reload when already leaving. Bubble Tea does not join command goroutines on exit, so quitting before the write completes could race application persistence cleanup. Read-only reloads retain existing immediate-quit behavior. Test this ordering with a deferred command and observable completion.
- [x] Preserve the existing request-generation protection and invalidate pre-mutation reads; older result/error messages cannot restore stale rows after a mutation/reload. Accept mutation completion only for the active operation; replaying a completion must not schedule another reload. Use explicit result messages and clear state names, not generic runner wrappers.
- [x] Mutation failures keep the existing rows/selection and display the sanitized error. Do not report success or reload on failure; release the pending state so retry/navigation/filter/quit work. A follow-up load error must be visible and keep the model usable without replaying the mutation.
- [x] A failed write when quitting must survive alternate-screen restoration: expose a final dashboard error for the CLI to join with program/cleanup errors, print through existing CLI error reporting, and exit nonzero. No generic runner wrappers or test-only production hooks. Test the dashboard's failed pending-quit error and real CLI error/status propagation.
- [x] Recalculate the viewport when action status changes content height, even without a resize; a visible selection in an 80x2 terminal stays visible during saving and after failure. Add a direct transition regression.
- [x] Honor remapped/disabled ToggleDone/Delete/Confirm/Cancel keys. Default old bindings must not stay active when replaced. While editing a filter, printable action keys remain text. Keep non-text quit behavior and current filter precedence. Clone newly used key slices like existing ones.
- [x] Test duplicate titles (right ID), repeated keys while pending, confirm/cancel/quit collisions, cancellation/no side effects, failures and retry, active-filter reload, stale load errors/results and duplicate completions, no selection, and pasted text not triggering destructive actions. Include long/unsafe titles/error text, Unicode, and zero/tiny/narrow resize during confirmation/pending/error; rendered output must remain within dimensions and prompts/errors must be visible when space permits. Footer help must match the active mode and bindings.
- [x] Keep all persistence rules/timestamps/tombstones inside `TodoService`. No infra imports, new dependencies, sync lifecycle, completed expansion, forms, or view switching in this PR. Completion disappears into the existing collapsed DONE section; reopening completed todos comes with its expansion slice.
- [x] Update old read-only test expectations to reflect implemented interactions without weakening existing navigation/filter coverage. Run focused UI tests while iterating, then `make audit` and `make build/psst` before commit. Self-review and report RED/GREEN commands/output, changed files, and any concerns. Independent review and terminal smoke verification follow before publication.

### Slice 4: Shared new/edit form with form routing

## Task 4: Dashboard create/edit form

**Files:** create `internal/ui/root.go`, `internal/ui/form.go`, `internal/ui/form_view.go`, `internal/ui/form_test.go`, and `internal/ui/root_test.go`; modify `internal/ui/dashboard.go`, `internal/ui/dashboard_test.go`, `internal/cli/cli.go`, and `internal/cli/cli_test.go`. Keep minimal-list routing for its own later slice.

**Interfaces:** Keep the dashboard on its existing `DashboardTodos` interface (`List`, `ToggleDone`, `Delete`). Define `FormTodos` in the form consumer package with exactly `List(string) ([]*domain.Todo, error)`, `Create(service.CreateTodoParams) (*domain.Todo, error)`, and `Update(string, service.UpdateTodoParams) (*domain.Todo, error)`; the UI must not import infra. Add `NewRoot(dashboardTodos DashboardTodos, formTodos FormTodos, theme config.ThemeConfig, keys config.KeysConfig, now time.Time, defaultList, defaultTime string) Root`; `Root` implements `tea.Model` and exposes unresolved final errors for CLI reporting. The CLI passes the same `App.TodoService` to both ports.

- [x] Write behavior-first tests for the form, root routes, and CLI integration before production changes; run and record expected RED. Use a real `TodoService` with a small in-memory `TodoStore` for mutation behavior, and keep bridge/infra out of tests.
- [x] Implement root routing for only Dashboard and Form. Dashboard's configured `new` and `edit` bindings emit request messages; Edit carries the selected todo ID. Keep the dashboard model intact while the form is open so cancel/success returns to its current applied filter and selection; success returns a deferred dashboard reload.
- [x] Implement one form model for Create and Update with fields in this order: title, natural-language deadline, description, list name. Tab/Shift+Tab changes focus; Esc or configured Cancel returns without a write; Enter validates/submits. Editing loads the selected todo asynchronously and initializes all fields.
- [x] Parse nonempty deadlines with `dateparser.Parse` using the form's injected reference time and `default_time`; blank clears/removes the deadline. Show live parse feedback and validation errors inline. Submit through `TodoService.Create`/`Update` so title/list/future-deadline validation and timestamps stay in the service. Do not call `CaptureService` or trigger Reminders sync.
- [x] Refresh list-name suggestions asynchronously every time a form opens, deriving unique names from `List("")` plus the configured default. Use fuzzy subsequence matching. Choosing a suggestion fills the field; typing a new nonempty list name remains valid.
- [x] Return write failures to the form, show sanitized errors, and allow correction/retry. Preserve unresolved write errors through cancel/navigation so `Root.Err()` lets existing CLI error handling report failure on quit; clear the error when an accepted retry succeeds/starts. Generation-tag async load/save messages so stale responses cannot replace the active screen.
- [x] Build form/root styles from `ThemeConfig`; keep View pure, escape/flatten terminal control text, and bound every rendered line to terminal width/height, including tiny windows. Test Unicode, pasted text, disabled/remapped keys, date parsing boundaries, invalid fields, error dismissal/retry, duplicate titles, filtered edits, cancel without side effects, failure, and no external Reminders access.
- [x] Update root-command tests to use `NewRoot`; preserve version, quick-capture, cancellation, startup errors, database cleanup, and the alternate-screen failed-write test. Run focused tests, then `make audit` and `make build/psst` before commit. Independently review the focused diff and exercise create/edit/cancel against a temporary database in a terminal.
- [x] Keep list suggestions limited to locally stored todo list names and the configured default in this slice. Do not add a remote-list cache, sync triggers/status, minimal list view, weekly progress, or completed expansion; those remain separate slices.

### Remaining slices

1. Add the minimal-list view to the root model and add configurable view switching.
2. Sync lifecycle/status, launch/full/manual sync, periodic pull, mutation-triggered debounce, shutdown coordination and refresh messages.
3. Weekly progress, completed-section expansion and remaining visual polish against the approved theme.

These are separate reviewable PRs. Slice 3 contains local dashboard actions only. Task 4 adds just the root routing needed for its form; MinimalList stays in the following slice.

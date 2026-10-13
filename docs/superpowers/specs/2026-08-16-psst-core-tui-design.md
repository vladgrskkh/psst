# psst — Core App + TUI Design (Spec 1)

Date: 2026-08-16
Status: Approved

## 1. Overview

psst is a macOS TUI for quick todo capture that two-way syncs with native macOS Reminders. The primary use case: you're in nvim, you need to note something, you capture it in seconds, and it lands in Reminders.app — which delivers the native notification when due.

This spec covers **spec 1: the core app + TUI**. The nvim plugin (spec 2) and optional sync daemon (spec 3) are deliberately out of scope and get their own spec → plan → implementation cycles.

## 2. Goals

- One-line quick capture: `psst new "buy milk @tomorrow 9:00 #groceries"` — no TUI needed.
- Full TUI with two views: **dashboard** (grouped by due date, progress bar) for everyday use and **minimal zen** (flat list) for lightweight browsing. Switchable at launch and in-app.
- Two-way full CRUD sync with macOS Reminders (create, complete, edit, delete) with deterministic conflict resolution.
- Beautiful by default: Catppuccin Mocha palette, fully overridable via config.
- Vim bindings by default, remappable via config.
- Local-first: everything works offline; sync catches up when it can.

## 3. Design principles

- **ISP (interface segregation)**: every consumer depends on a narrow interface; implementations live in infra and are wired only at the composition root.
- **Clean architecture layering**: `pkg/option + domain ← service ← cli/ui`; `app` composes service ports with infra adapters.
- **DRY**: one capture form for new/edit, one Option type, one theme builder, one choke point for mutations.
- **Elm architecture in the TUI** (Bubble Tea paradigm): Model (state), Update (message → state + commands), View (state → rendering). Update functions stay pure: service calls are returned as `tea.Cmd`s that produce messages.
- **Local-first**: the local store is the source of truth at runtime; Reminders is a sync target.

## 4. Layering & project layout

```
cmd/psst/main.go        — thin process entry point
internal/cli/           — Cobra commands and CLI presentation
internal/app/           — composition root: loads config and wires infra → service
internal/domain/        — Todo, sync state, and sync snapshot domain types
pkg/option/             — reusable generic Option[T] package
internal/service/       — business logic: TodoService, sync engine, narrow ports
internal/infra/         — implementations of service ports:
    store/              — bbolt TodoStore
    reminders/          — Swift bridge client (+ Swift source in macosbridge/)
internal/config/        — config + theme loading
internal/dateparser/    — natural date parsing
internal/quickcapture/  — one-line quick-capture grammar
internal/ui/            — TUI presentation (root, dashboard, list, form, theme)
```

Dependency rule: `pkg/option + domain ← service ← cli/ui`; `app` wires service
and infra adapters. CLI and UI never import infra directly.

## 5. Data model

```go
// pkg/option/option.go — dependency-free generic wrapper; JSON-marshalable for bbolt
type Option[T any] struct { ... }
func Some[T any](v T) Option[T]
func None[T any]() Option[T]
func (o Option[T]) Get() (T, bool)
func (o Option[T]) IsPresent() bool

// domain/todo.go
type Todo struct {
    ID             string              // psst's own uuid
    ReminderID     option.Option[string] // EventKit identifier; None until first push
    ListID         option.Option[string] // resolved Reminders list identifier; None until resolved (pushed by name)
    ListName       string              // always set: from picker or default_list; used to resolve ListID at push time
    Title          string
    Description    string              // "" is a real value
    Deadline       option.Option[time.Time] // None = no due date ("later")
    Done           bool
    CompletedAt    option.Option[time.Time]
    CreatedAt      time.Time
    UpdatedAt      time.Time
    Deleted        bool                // tombstone for sync
    DeletedAt      option.Option[time.Time]
    RemoteSnapshot string              // JSON of last-known Reminders state (conflict detection)
    LocalSnapshot  string              // JSON of local fields at last successful sync (conflict detection)
}
```

No pointer fields to express emptiness — `Option[T]` everywhere absence is meaningful.

## 6. Storage

- bbolt database at `~/.local/state/psst/psst.db` (path overridable via config).
- Buckets: `todos` (keyed by ID), `meta` (sync bookkeeping: `lastPullAt`, `lastPushAt`, cached lists, bridge version).
- Serialization: JSON (fixes the broken gob code in the current repository).
- Interfaces (in `service`, implemented in `infra/store`):
  - `TodoReader`: `Get(id)`, `List()`, `ListByReminderID(rid)`
  - `TodoWriter`: `Put(todo)`, `MarkDeleted(id)`, `PurgeTombstones(before)`
  - `SyncMetaReader` / `SyncMetaWriter`: `GetSyncState()`, `SaveSyncState()`
- Open/close lifecycle managed at composition root; a `*bolt.DB` is never leaked into service or UI.

## 7. Reminders bridge

A small Swift CLI (`macosbridge/`) using EventKit, compiled by the Makefile (`swiftc`) into the same directory as the psst binary.

Protocol: line-delimited JSON over stdin/stdout.

```json
{"id": 1, "method": "list-lists", "params": {}}
{"id": 1, "result": {"lists": [{"id": "...", "name": "..."}]}}
{"id": 1, "error": {"code": "access-denied", "message": "..."}}
```

Methods:

| Method | Params | Result |
|---|---|---|
| `permissions` | — | `{status: "authorized"\|"denied"\|"not-determined"}` |
| `list-lists` | — | all Reminders lists with ids |
| `ensure-list` | `{name}` | `{id}` — returns the id of an existing list with that name, creating it if missing |
| `pull` | `{listIDs: []}` | reminders in those lists: `{id, listID, title, notes, dueDate?, completed, completionDate?}` |
| `create` | `{listID, title, notes, dueDate?}` | `{id}` |
| `update` | `{id, fields: {title?, notes?, dueDate?, listID?, completed?}}` | `{}` |
| `delete` | `{id}` | `{}` |

- Due dates are `time.Time` values rendered as RFC 3339; the bridge converts to `dueDateComponents` (datetime; time-less dates are not used — the parser always fills a default time).
- Go side: `RemindersGateway` port interface in `service`; `infra/reminders.Client` implements it over the bridge process (spawn on demand, reuse while running). Sync engine and tests depend only on the port.
- First bridge use triggers the macOS Reminders permission prompt (TCC). Denial is surfaced as a typed error, not a crash.

## 8. Sync engine

Lives in `service` (business logic), `Syncer` interface: `FullSync(ctx)`, `PushChanges(ctx)`, `PullChanges(ctx)`.

**Triggers:**
- On launch: `FullSync` (pull first, then push)
- On local mutation: debounced push (500ms)
- Periodic: pull every `sync_interval_seconds` (default 30) — catches edits made in Reminders.app
- Manual: `s` key forces a full sync

**Push algorithm:**
- Resolve lists: for todos with `ListID` absent, call `ensure-list` with `ListName` first.
- For each todo with `UpdatedAt > lastPushAt` (or never pushed):
  - `Deleted` tombstone with `ReminderID` → bridge delete; mark tombstone pushed.
  - `ReminderID` absent → bridge create; store returned id.
  - Otherwise → bridge update (title, notes, due date, list, completed).
- After successful push, update `RemoteSnapshot` and `LocalSnapshot` to the values just sent.

**Pull algorithm:**
- Fetch all reminders in tracked lists (lists referenced by any todo + `default_list`).
- Match by `ReminderID`:
  - Unknown reminder → create locally (new remote todo), snapshots set.
  - Known → per-field merge via snapshots:
    - `remoteChanged` = diff(`RemoteSnapshot`, fetched values)
    - `localChanged` = diff(`LocalSnapshot`, current local values)
    - For each field: remote-only change → apply; both changed → local wins; local-only → keep (will push).
    - Update `RemoteSnapshot` to fetched values and `LocalSnapshot` to resulting local values.
  - This is a deterministic per-field approximation of last-write-wins (EventKit exposes no modification timestamps).
- Reminders missing from the pull result → if `localChanged` is empty, mark local `Deleted` (tombstone); otherwise re-push.
- Update `lastPullAt`; emit `SyncStatusChanged` for the UI.

**Tombstones**: retained `tombstone_retention_days` (default 30), then purged.

## 9. Service layer

- `TodoService` — single choke point for mutations:
  - `Create(title, description, deadline Option, list)`, `Update(id, fields...)`, `ToggleDone(id)`, `Delete(id)`, `List(filter)`.
  - Validates (non-empty title; deadline if present must be in the future), stamps `UpdatedAt`/`CompletedAt` via a `Clock` port (injected; real time in prod, fixed in tests).
- `SyncService` — owns the engine, debounce, ticker; exposes `Status()` for the UI.
- All ports are tiny and segregated: `TodoReader`, `TodoWriter`, `RemindersGateway`, `Clock`, `SyncMetaReader`, `SyncMetaWriter`. Tests fake only what they need.

## 10. CLI

- `psst` — opens the dashboard TUI.
- `psst new "<text>"` — quick capture, no TUI:
  - Grammar: `@<date hint>` = due date, `#<list name>` = target list, everything else = title.
  - Example: `psst new "buy oat milk @tomorrow 9:00 #groceries"` → title "buy oat milk", due tomorrow 09:00, list "groceries".
  - Missing list → `default_list` from config. Missing `@` → no deadline.
  - Goes through `TodoService.Create` then `SyncService.PushChanges`; prints one confirmation line ("✓ created — due tomorrow 09:00, list groceries") or an error with a non-zero exit code.
- `psst --version`.

## 11. Config & theme

`~/.config/psst/config.toml` (override via `PSST_CONFIG`). Missing file = all defaults. Partial overrides allowed; unknown keys ignored with a warning.

```toml
[general]
default_list = "psst"          # created in Reminders if missing
sync_interval_seconds = 30
push_debounce_ms = 500
tombstone_retention_days = 30
default_time = "09:00"         # used when a date has no time part
db_path = ""                   # default ~/.local/state/psst/psst.db

[theme]                        # Catppuccin Mocha defaults; any subset overridable
bg = "#1e1e2e"
surface = "#313244"
border = "#45475a"
text = "#cdd6f4"
muted = "#6c7086"
accent = "#89b4fa"
success = "#a6e3a1"
warning = "#f9e2af"
danger = "#f38ba8"
overdue = "#f38ba8"
today = "#fab387"
tomorrow = "#89b4fa"
later = "#6c7086"
cursor = "#cba6f7"
progress = "#cba6f7"

[keys]                         # vim-style defaults; values are lists of key names
up = ["k", "up"]
down = ["j", "down"]
toggle_done = ["space", "x"]
new = ["n"]
edit = ["e"]
delete = ["d"]
confirm = ["y"]
cancel = ["esc"]
toggle_view = ["tab"]
filter = ["/"]
sync_now = ["s"]
quit = ["q", "ctrl+c"]
```

Theme is applied as lipgloss hex colors (truecolor; terminals without truecolor degrade gracefully). `ui/theme.go` builds all styles from tokens once; views consume styles, never hardcode colors.

## 12. TUI (Elm architecture)

**Root model**: `Screen` enum (`Dashboard`, `MinimalList`, `Form`) + child models. Root delegates `Update`/`View`; screen transitions are messages (`NewTodoRequested`, `EditTodoRequested`, `TodoCreated`, `BackRequested`, `ViewToggled`). Cleaned-up version of the existing `internal/ui/root/ui.go` routing.

**Update purity**: Update returns `tea.Cmd`s wrapping service calls (e.g. `TodoService.Create`) that produce messages (`TodoCreated`, `TodosRefreshed`, `SyncStatusChanged`, `ErrorOccurred`). State mutates only on messages.

**Dashboard** (`psst` default):
- State: todos, cursor, `grouped` flag, filter string, sync status.
- View: header (title + weekly progress bar `4/7`), sections OVERDUE / TODAY / TOMORROW / LATER / DONE (collapsed), colored group headers, `[ ]`/`[x]` checkboxes, cursor-row highlight, footer help bar.
- Keys: move / toggle done / `n` new / `e` edit / `d` delete (inline `y/n` confirm) / `tab` → minimal / `/` filter / `s` sync / `q` quit.

**MinimalList**:
- State: flat todos, cursor.
- View: zen card — thin rounded border, one accent color, muted dates, "3 done · 4 open" footer.

**Form** (one model for new + edit, different initial state — DRY):
- State: focus index; textinputs for title, date (natural language, live-parsed), description; list fuzzy-picker (cached lists, refreshed when opened); inline validation errors.
- Submit returns a create/update command; on success → `TodoCreated` → back to previous screen.

**Notifications**: Reminders.app delivers the due notification natively (psst sets `dueDate`). No in-app notifier; the `macnotify` package is removed.

## 13. Error handling

- **Local-first**: mutations hit the local store first; sync is async and never blocks the TUI.
- Bridge missing / permission denied / Reminders down → sync errors surface in the status line ("sync failed — retrying in 30s", "access denied → System Settings → Privacy → Reminders").
- Form validation inline (bad date hint, empty title). `ErrorOccurred` messages render as a transient banner.
- DB open failure at startup → message + exit 1 (nothing can work without the store).

## 14. Testing

- **Unit**: dateparser tables (hints, edge cases); Option JSON round-trip; TodoService with fake store + fixed Clock (validation, timestamping); sync engine with fake `RemindersGateway` (create/update/complete/delete/conflict/tombstone scenarios); CLI grammar parser.
- **TUI**: bubbletea test harness — feed `tea.KeyMsg` sequences, assert state transitions and rendered output for create / toggle / delete / navigate / view-toggle flows.
- **Bridge**: Swift side is a thin EventKit wrapper (manual test checklist against real Reminders); Go client unit-tested against a fake bridge process (scripted JSON lines).

## 15. Repo cleanup (part of spec 1)

- Fix `infra/store`: bucket-name mismatch (`Notifications` vs `Notification`), broken gob encode/decode (replace with JSON).
- Fix `internal/ui/models/allnotifications.go` (references undefined `m.notifications`) — superseded by the new TUI.
- Remove `internal/macnotify`.
- Move `my.db` and `debug.log` out of the repo root to `~/.local/state/psst/` (`psst.db`, `psst.log`); log level configurable.
- Delete the old tutorial UI (`internal/ui/root`, `internal/ui/models`) once the new UI replaces it.

## 16. Out of scope (later specs)

- **Spec 2 — nvim plugin**: floating-terminal launcher (dashboard + minimal capture form), one-line capture mapping, picker integration (telescope, fzf-lua, mini.pick) via an abstract picker adapter.
- **Spec 3 — daemon mode**: optional background daemon owning the DB + instant sync; TUI/CLI become thin clients over a unix socket. The sync engine from spec 1 is designed to be reused unchanged inside the daemon.

## 17. Success criteria

- `psst new "buy oat milk @tomorrow 9:00 #groceries"` creates a reminder visible in Reminders.app within seconds; deleting/toggling it in Reminders.app reflects in psst on next pull (≤30s).
- Dashboard and minimal views both fully functional with Catppuccin theme; theme tokens overridable in config.
- Vim bindings work by default and can be remapped in config.
- Offline capture works; sync reconciles on next successful connection.
- `make audit` passes (vet, golangci-lint, tests).

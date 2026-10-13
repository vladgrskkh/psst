# AGENTS.md

Instructions for AI agents working on this repository.

## Project

psst is a macOS TUI + CLI for quick todo capture with two-way sync to native
macOS Reminders (via a Swift EventKit bridge). Use case: you're in nvim, note
something in seconds, it lands in Reminders.app which fires the native
notification when due.

- Go 1.25.4, module `github.com/vladgrskkh/psst`
- TUI: Bubble Tea (Elm architecture — Model / Update / View)
- Storage: bbolt (JSON serialized)
- macOS bridge: Swift/EventKit binary (`psst-bridge`), line-delimited JSON over stdin/stdout
- nvim plugin and sync daemon are later specs — not in this branch

## Architecture (must not be violated)

```
cmd/psst/              thin process entry point
internal/cli/          Cobra commands and CLI presentation
internal/app/          composition root; loads config and wires infra to services
internal/domain/       pure domain types (Todo, TodoSnapshot, SyncState)
internal/service/      business logic (TodoService, sync engine) + narrow ports
pkg/option/            reusable generic Optional value exposed for importers
internal/infra/        port implementations: store/ (bbolt), reminders/ (bridge client)
internal/config/       config + theme loading
internal/dateparser/   natural date parsing
internal/quickcapture/ one-line quick-capture grammar
internal/ui/           TUI (plan 2 — not present yet)
```

Dependency rule: `pkg/option + domain ← service ← cli`; `app` wires service and
infra implementations. The CLI and UI use app/service APIs and do not import
infra packages directly.

## Conventions

- **ISP**: every consumer depends on the narrowest interface it needs. Ports live in `service`; errors a gateway consumer must handle (e.g. `service.ErrReminderNotFound`) live next to the port, never in infra.
- **Clean code / DRY**: one choke point for mutations (TodoService), one form for new/edit, no hardcoded colors (theme tokens).
- **No pointers for optionality**: use `option.Option[T]` (`Some`/`None`); `Option[Option[T]]` means None=don't touch, Some(None)=clear.
- **CLI**: define commands with Cobra; keep `cmd/psst/main.go` limited to startup and exit status.
- **Local-first**: mutations hit the local bbolt store first; sync is async and never blocks the CLI/TUI.
- **Sync model**: per-field two-way merge via `RemoteSnapshot`/`LocalSnapshot` JSON on each Todo; local wins on true conflicts (deterministic LWW approximation).
- **Testing**: TDD, stdlib `testing` only (no frameworks). Tests are table-driven where sensible.
- **Commits**: conventional style (`feat(domain): ...`). Never commit secrets.

## Task decomposition and review

- Keep each PR focused on one independently reviewable outcome. Split larger
  features into smaller PRs with explicit dependencies; use stacked PRs when
  a later change requires an earlier one.
- Separate prerequisite refactors from behavior changes when each can compile
  and pass its checks independently. Avoid combining unrelated subsystems in
  one implementation batch.
- Prefer bounded subagent implementation tasks with disjoint file ownership
  and clear acceptance criteria. Use independent subagent review for meaningful
  changes, and integrate and verify the resulting patches before publication.
- Keep work on the immediate critical path local while subagents handle useful
  independent tasks. Do not delegate duplicate work or add agents merely to
  increase the number of agents.

## Commands

```bash
make audit          # go mod tidy/verify, fmt, vet, golangci-lint, go test -race ./...
make build/psst     # builds ./bin/psst
make build/bridge   # swiftc -> ./bin/psst-bridge (Swift/EventKit, macOS only)
make run/psst       # go run ./cmd/psst
```

`make audit` must pass at the end of every task.

## Runtime paths

- Config: `~/.config/psst/config.toml` (override: `PSST_CONFIG`) — defaults: Catppuccin Mocha theme, vim keybindings, sync every 30s
- State: `~/.local/state/psst/` (`psst.db`, `psst.log`)

## Design docs

- Spec: `docs/superpowers/specs/2026-08-16-psst-core-tui-design.md`
- Implementation plan (core): `docs/superpowers/plans/2026-08-16-psst-core.md`

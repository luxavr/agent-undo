# Agent Undo — engineering rules

Ctrl+Z for a wrapped terminal AI coding session. Local-first checkpoint, session, restore. Go only in the core.

Read [docs/v0.1-contract.md](docs/v0.1-contract.md) for the frozen public behavior. Read [docs/prd-v2.txt](docs/prd-v2.txt) for product scope. Read [docs/analysis.md](docs/analysis.md) before changing architecture, CLI surface, restore, or git policy. Read [docs/architecture.md](docs/architecture.md) before adding packages. Read [docs/security-model.md](docs/security-model.md) before any path, symlink, lock, or spawn work. Read [docs/adr/0002-git-restore-policy.md](docs/adr/0002-git-restore-policy.md) before any git write. Read [docs/adr/0003-ignore-policy.md](docs/adr/0003-ignore-policy.md) before changing capture. Read [docs/adr/0004-session-state.md](docs/adr/0004-session-state.md) before changing `run`, session records, exit codes, or CLI target ids.

The checkpoint is the source of truth. Git is one dimension of state, not the whole state.

## Priorities

1. Data safety
2. Restore correctness
3. Verification
4. Crash/interruption recovery
5. Security boundaries
6. Simple architecture
7. CLI UX
8. Performance
9. Extensibility

## Non-negotiables

- Go for core and CLI. No Node.js in the runtime.
- Never destructive-restore without a recovery checkpoint first.
- Restore order: LOCK → LOAD → VALIDATE → PLAN → RECOVERY CHECKPOINT → APPLY → VERIFY → REPORT.
- Print success only after verify.
- Fail closed. No silent restore failures. No “100% reversible” for external side effects.
- Standard library first. No global mutable state. `context.Context` on cancellable work.
- Destructive filesystem logic stays in `internal/restore` and `internal/security`.
- One batch at a time. Do not implement later roadmap (policy, cloud, firewall, risk engine).

## CLI (canonical)

`run`, `session list`, `session show`, `undo`, `verify`, `diff`, `recover`, `doctor`.

Canonical `undo` / `verify` / `diff` / `session show` take a **session id** (`XXXXXXXX-XXXXXXXX`). Raw checkpoint ids (`cp_…`) are rejected at the CLI.

Undo requires the wrapped process to have started (`StartErr == nil`). `recover --yes` requires an explicit `cp_…`. `run` refuses when cwd is the user home directory; `doctor` warns there.

`session show` = receipt/metadata. `diff` = file/state delta. `recover` = recovery checkpoint. `keep` is not a command.

Do not add `init` or `inspect`. Ignore leftover `npm/pnpm` Batch 0 text in the PRD.

## For coding agents

Agent Undo is a user-level CLI.

Do not:

- run `agent-undo init`
- modify the user's `.gitignore` for Agent Undo
- create `.agent-undo/` inside the repository
- treat `doctor` as repository setup

Do:

- inspect the current directory with `agent-undo doctor`
- start protection with `agent-undo run <agent>`

This source tree's `.gitignore` entry for `.agent-undo/` is contributor hygiene, not user installation. The store is `~/.agent-undo/` or `$AGENT_UNDO_HOME`. v0.1 is not a sandbox.

## Loop

1. Inspect what exists. No duplicate abstractions.
2. Tests first for filesystem and path logic.
3. Smallest coherent change.
4. `gofmt`, `go test ./...`, `go test -race ./...`, `go vet`.
5. Exercise the failure path.
6. Report: objective, files, tests, commands, limitations, security, next batch.

Done means the batch acceptance criteria are met and tested, not that it compiles.

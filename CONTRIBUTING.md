# Contributing

Go only in the core and CLI. No Node.js in the runtime.

## Before a change

Read `AGENTS.md`, `docs/architecture.md`, and the ADR that owns the area (git writes → ADR 0002, capture → ADR 0003, paths → `docs/security-model.md`).

## Loop

1. Tests first for filesystem and path logic.
2. Smallest coherent change.
3. `gofmt -w .`
4. `go test ./...`
5. `go test -race ./...`
6. `go vet ./...`

## Do not

- Implement restore APPLY until Batch 3
- Add `inspect` / `init` as CLI verbs
- Put fake command or risk telemetry on the receipt
- Silently rewrite git history

## Platforms

Develop on macOS or Linux. Windows is not a v0.1 target.

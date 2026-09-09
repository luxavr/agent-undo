# Contributing

Go only in the core and CLI. No Node.js in the runtime.

Public first-run install is a GitHub Release binary. This file is the **Go** path: people who already have a toolchain.

## Install from source

Go 1.23+. Put `$(go env GOPATH)/bin` on `PATH` if you use `go install` (that directory is not on PATH by default).

```bash
go install github.com/luxavr/agent-undo/cmd/agent-undo@v0.1.0
```

Unreleased tip of `main`:

```bash
go install github.com/luxavr/agent-undo/cmd/agent-undo@main
```

Unreleased checkout:

```bash
git clone https://github.com/luxavr/agent-undo.git
cd agent-undo
go build -o bin/agent-undo ./cmd/agent-undo
```

Untagged builds report `0.0.0-dev`. A release build injects the tag:

```bash
go build -trimpath -ldflags "-X github.com/luxavr/agent-undo/internal/version.Version=v0.1.0" -o bin/agent-undo ./cmd/agent-undo
```

Do not upload locally built binaries as a GitHub Release. Tags matching `v*` publish through `.github/workflows/release.yml`, which runs `scripts/build-release.sh`.

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

- Add `inspect` / `init` as CLI verbs
- Put fake command or risk telemetry on the receipt
- Silently rewrite git history
- Add Homebrew, `curl | sh`, or Windows support in v0.1
- Implement Batch 8 subsystems

## Platforms

Develop on macOS or Linux. Windows is not a v0.1 target.

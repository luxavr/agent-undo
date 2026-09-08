# 0001. Go for the Agent Undo core and CLI

## Status

Accepted (PRD v2, explicit).

## Context

v1 left the stack as TypeScript or Go. The workload is filesystem-heavy, signal-sensitive, and safety-critical. Distribution should be a single binary with no runtime for the user to install.

## Decision

Implement the core and CLI in Go. TypeScript is reserved for a future web, dashboard, or SDK surface. Node.js must not enter the core runtime.

## Consequences

- Batch 0 means `go.mod`, not `package.json`. The PRD Batch 0 “TS config OR Go modules” / “npm/pnpm install works” lines are stale; ignore them. TypeScript is not part of the core implementation plan.
- CI and contributor workflow: `gofmt`, `go vet`, `go test ./...`, `go test -race ./...`.
- v0.1 platforms: macOS and Linux. Windows is a documented gap, not a reason to change this ADR.
- Contributors need Go, not Node, to hack on undo/restore.

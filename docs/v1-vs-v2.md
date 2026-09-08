# PRD v1 → v2

v2 is the same product with a locked implementation language and a stricter Cursor brief. Most sections are unchanged copy.

## Real changes

1. **Go is mandatory.** v1 allowed TypeScript or Go and recommended TS for speed. v2: Go for core and CLI; TypeScript only for a future web/SDK; no Node in the runtime.
2. **Repo layout** matches Go (`cmd/`, `internal/`, `adapters/`) instead of `src/`.
3. **Tooling/CI** named: gofmt, vet, `go test -race`, fuzz for path/manifest, cross-platform matrix.
4. **Package principles** added: small public API, stdlib first, `context.Context`, isolated destructive IO, no CLI leaked into checkpoint/restore.
5. **Cursor implementation brief** expanded: ordered engineering priorities, explicit restore pipeline, security requirements, test requirements, batch report format.
6. **`adapters/agents`** appears in the tree. Still no requirement to build named adapters in MVP.

## Leftovers (v1 text that v2 did not clean)

- Batch 0 still says “TS config OR Go modules” and “npm/pnpm install works.”
- Some command lists still include `init` / `status` / `sessions` / `inspect` from the v1 CLI sketch.

When the two documents disagree on stack, **v2 §9 / §49 win**. When they disagree on CLI names, **v2 §15 wins**.

v0.1 lock (do not re-litigate in code): Go only; canonical CLI `run` / `session list` / `session show` / `undo` / `verify` / `diff` / `recover` / `doctor`; recovery checkpoint is P0; receipts are captured facts only. See [architecture.md](architecture.md).

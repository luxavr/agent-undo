# Changelog

## Unreleased

### Added

- Architecture lock: ADRs 0002 (git restore) and 0003 (ignore/capture).
- Batch 0: Go module, canonical CLI (help/version; other commands parse and return not-implemented).
- Batch 1 snapshot engine: repository boundary, ignore classifier, content-addressed store, verified checkpoints. No restore APPLY.
- Batch 2: deterministic `diff.Compare` (added/modified/deleted/type-changed/symlink-changed/unchanged). No rename inference.
- Batch 3: restore pipeline with recovery checkpoint, git HEAD move + overlay, verify-owned success. CLI `undo [--yes]` and `verify`.
- Batch 4: session wrapper (`run`): repository lock, verified checkpoint, process group, inherited stdout/stderr, final manifest, `diff.Compare`, persisted session record. CLI `session list|show` and `diff`. Commands take session ids, not raw `cp_…`.
- Batch 5: honest receipt (`run` / `session show`): checkpoint integrity, undo availability, standing undo scope, next commands. Projection of persisted artifacts only. `diff` remains path-level.
- Batch 6: `doctor` readiness report. Status READY / READY WITH WARNINGS / NOT READY. Lock probe (acquire+release). Bounded class walk. No repair.
- Batch 7: `recover [--yes] [cp_…]`. Latest valid recovery checkpoint for this repository, or explicit `kind: recovery` id. Same restore engine. Optional lineage `source` on recovery manifests.
- v0.1 feature freeze: public contract `docs/v0.1-contract.md`, canonical demo `examples/`, pre-launch audit, GitHub issue taxonomy. Overlay restores a file even when the live path is a directory with uncaptured children.
- Public identity: module and install path `github.com/luxavr/agent-undo`. README reordered for first-60-seconds. Canonical demo capture in `docs/demo-capture.txt` / `docs/demo.gif`.
- Distribution pass: GitHub Release workflow (binaries after `v0.1.0`), `internal/version` ldflags, `doctor` prints `checking: <path>`. Until the tag, public install is `go install …@main`; README does not advertise a download URL that 404s.

### Changed

- Undo is allowed only when the wrapped process started (`StartErr == nil`). LookPath and `cmd.Start` failures keep a session checkpoint but are not undoable. Started + exit 2, SIGINT, and `FINAL_SNAPSHOT_FAILED` stay undoable. ADR 0004 command matrix updated.
- `recover --yes` without an explicit `cp_…` is refused. Interactive `recover` with no id may still select the latest recovery and confirm. Confirmation may be skipped; target selection may not.
- `run` refuses when the cleaned working directory equals the user home directory. `doctor` from `$HOME` stays inspection-oriented and warns that `run` will refuse. Nested project directories and `/tmp` are unchanged.

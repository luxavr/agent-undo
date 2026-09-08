# Architecture

Public contract: [v0.1-contract.md](v0.1-contract.md). Decisions: [architecture-decision.md](architecture-decision.md), [adr/0002-git-restore-policy.md](adr/0002-git-restore-policy.md), [adr/0003-ignore-policy.md](adr/0003-ignore-policy.md), [adr/0004-session-state.md](adr/0004-session-state.md). Safety: [security-model.md](security-model.md).

**The checkpoint is the source of truth. Git is one dimension of state, not the whole state.**

## Layout

```
cmd/agent-undo/          main: os.Args → internal/cli
internal/
  cli/                   help, version, command routing
  ignore/                capture classifier (ADR 0003)
  checkpoint/            traverse, hash, write objects, manifest, completeness verify
  restore/               pipeline: LOCK → LOAD → VALIDATE → PLAN → RECOVERY CHECKPOINT → APPLY → VERIFY → REPORT
  diff/                  checkpoint-to-checkpoint file delta
  session/               session id, record, wrap orchestrator (ADR 0004)
  verify/                workspace vs manifest; owns the word "success"
  storage/               $AGENT_UNDO_HOME or ~/.agent-undo
  process/               direct exec, process group, SIGINT→TERM→KILL
  security/              repository boundary, traversal, symlink rules
  doctor/                install/repository readiness (no repair, no crawl)
adapters/
  git/                   read HEAD/branch/tracked/ignored; write only after ADR 0002 + restore Apply
  filesystem/            lstat/read/create within a Boundary
  shell/                 not in v0.1 receipts
  agents/                named argv helpers later; wrap is process.Run
```

`checkpoint`, `restore`, and `verify` must not import `cli`. Adapters do not implement path policy; they call `internal/security`.

## Repository boundary

The workspace root: canonicalized directory the user invoked from, or the git top-level if `.git` exists **and** that top-level is an ancestor of cwd.

All captured paths are relative to this root, with `/` separators in the manifest. Restore writes only through `security.Boundary`.

Git is optional. No `.git` → filesystem checkpoint only.

## Filesystem-state model (MVP)

A checkpoint manifest (`schemaVersion: 1`) records:

- `id`, `kind` (`session` | `recovery` | `final`), `repoRoot`, `createdAt`
- `git`: `{ captured, head, branch, detached }` or `captured: false`
- `files[]`: `{ path, kind: file|symlink|dir, mode, size, sha256?, linkTarget? }`
- `exclusions[]`: `{ class, count }` — classes, not path dumps
- `source` (optional, recovery only): `{ type: undo|recover, sessionId?, checkpointId? }` — lineage; APPLY and VERIFY ignore it

Blobs live in the object store, keyed by SHA-256 of file bytes. Symlinks have no blob. Directories have no blob.

A checkpoint is **complete** only if every file entry’s blob is present and hashes, and the manifest itself verifies. Incomplete → not restorable.

## Store

```
$AGENT_UNDO_HOME or ~/.agent-undo/
  objects/aa/bb/<sha256>     content-addressed regular-file bytes
  manifests/<id>.json
  sessions/<session-id>/record.json
  locks/<boundary-hash>
```

Atomic create: write temp + `fsync` + rename. On read, rehash. Mismatch → unusable, not best-effort.

## Session lifecycle

States (ADR 0004): `CREATED` → `CHECKPOINTING` → `RUNNING` → `COMPLETED` | `INTERRUPTED` | `FAILED`.

Outcomes are orthogonal: `CHILD_EXIT`, `INTERRUPTED`, `AGENT_UNDO_ERROR`, `FINAL_SNAPSHOT_FAILED`, `CHECKPOINT_FAILED`.

`agent-undo run <cmd>`:

1. Resolve boundary. Acquire the single repository lock (fail closed if held).
2. Create and **verify** a session checkpoint (`kind: session`). On failure: `FAILED` / `CHECKPOINT_FAILED`, release lock, exit 3.
3. Spawn argv directly (no `sh -c`). Child gets its own process group. stdout/stderr inherit. Set `AGENT_UNDO_SESSION=<id>` as a marker only; the lock is authoritative.
4. On exit or interrupt: signal group (SIGINT → grace → SIGTERM → grace → SIGKILL), wait, then create a **final** manifest with the same walker. `diff.Compare(before, after)`. Persist the session record. Release the lock.
5. Print the receipt (same as `session show`). Return the child's exit status when `COMPLETED`. Interrupted after the agent started → 130. Agent Undo internal failure → 3.

Exit status alone does not attribute failure. Use the session record (`state`, `outcome`, `process.exit_code`, `agent_undo.error_code`).

Canonical CLI (`undo`, `verify`, `diff`, `session show`) takes a **session id**. Raw `cp_…` ids are rejected. Tests may call `restore.Run` with a checkpoint id.

`keep` is not a command. The receipt prints next-step commands (`session show`, `diff`, `undo` when allowed).

v0.1 is wrapper-based. Cursor/editor-native attachment is not supported.

## Doctor

`agent-undo doctor` reports whether this environment can keep Agent Undo's promises. It does not repair, hash, snapshot, or steal a lock.

Status (standing limitations do not set it):

- `READY` — required capabilities are available
- `READY WITH WARNINGS` — non-blocking findings (exclusion classes, external symlink class, lock currently held)
- `NOT READY` — cannot establish boundary, store, writable probe, lock probe, or supported OS

Exit: `READY` / `READY WITH WARNINGS` → 0; `NOT READY` → 3; usage → 1.

Lock probe: acquire → write PID marker → immediately release. Held flock is a warning with pid. Doctor never deletes lock files.

Filesystem inspection is bounded (`MaxWalkDepth` 3, `MaxWalkVisit` 4096): skip excluded trees immediately, classify classes not paths, do not hash or read file contents. `.git` is not a warning finding (Git section). Unreadable workspace root → `NOT READY`; capped or partial discovery → warning.

Support lines (index fidelity, external side effects, no Cursor attach) are always printed as notes.

## Restore state machine

Legal order; no stage skipped:

```
LOCK → LOAD → VALIDATE → PLAN → RECOVERY CHECKPOINT → APPLY → VERIFY → REPORT
```

| Stage | Contract |
|---|---|
| LOCK | Exclusive repo lock. No overlapping `run`, `undo`, or `recover`. No silent steal of a held flock (`doctor` diagnoses). A leftover `.lock` file is not a lock. |
| LOAD | Read manifest + objects. Untrusted until hashed. |
| VALIDATE | Same canonical root; schema; object integrity; if `git.captured`, `.git` present and captured SHA in the object DB. |
| PLAN | Show CAPTURED / SESSION / PROPOSED git refs; session commits; branch changes; pre-session dirty overlay; deletes; unreachable-from-restored-branch; recovery hint. TTY confirm unless `--yes`. |
| RECOVERY CHECKPOINT | Verified checkpoint of **current** post-agent state. Mandatory. Failure → abort, no APPLY. |
| APPLY | Explicit paths only. Git: checkout captured branch or detached SHA, `reset --hard` captured SHA, **then** overlay checkpoint (authoritative for dirty/untracked/captured-ignored files). Never force-push, delete branches, or prune. Git index/staging is not restored. |
| VERIFY | Full recompute vs target checkpoint. Write count is not success. VERIFY owns SUCCESS / FAILED / INCOMPLETE. |
| REPORT | May say incomplete. Must not say success unless VERIFY passed. |

Recovery checkpoint capability is P0. `agent-undo recover [cp_…]` restores a `kind: recovery` checkpoint for the current boundary via the same restore engine. No-argument lookup is read-only and happens before `restore.Run`; after lock the engine re-LOAD/VALIDATE the selected id and fail closed. It does not switch targets. Lineage (`source`) is optional metadata.

Invariant:

```
CURRENT STATE
    ↓
CREATE + VERIFY RECOVERY CHECKPOINT
    ↓
APPLY TARGET RECOVERY CHECKPOINT
    ↓
VERIFY
```

## CLI (canonical)

```
agent-undo run <agent command>
agent-undo session list
agent-undo session show <session-id>
agent-undo undo <session-id>
agent-undo verify <session-id>
agent-undo diff <session-id>
agent-undo recover [cp_…]
agent-undo doctor
```

- `session show` = receipt / metadata
- `diff` = file/state delta
- `recover` = restore a recovery checkpoint
- No `init`, `inspect`, `status`, `sessions` as verbs

Receipt fields are a **projection of persisted artifacts** (session record, checkpoint, final manifest, `diff.Compare`). `run` and `session show` print the same receipt: session id, command, duration, agent result, file counts, git refs or `not captured`, checkpoint integrity (`VERIFIED` / `UNAVAILABLE`), undo availability, standing undo scope, and next commands. No `[k][i][u]` loop. `diff` owns path-level delta. `verify` / `undo` own SUCCESS / FAILED / INCOMPLETE. No command counts, package telemetry, or risk scores.

## Out of scope (v0.1)

Named agent adapters, shell command observation, policy/risk engine, cloud, IDE attach, Windows support, watchers / created-file “ownership”, `node_modules` restore, reversing remotes or anything off-box.

## Implementation gates

| Batch | May write |
|---|---|
| 0 | Module, CLI parse, help/version, CI |
| 1 | Boundary, ignore, store, checkpoint (read git only) |
| 2 | Diff two manifests (`diff.Compare`); no rename inference |
| 3 | Restore APPLY with recovery checkpoint + ADR 0002; CLI `undo`/`verify` |
| 4 | Session wrapper: lock → checkpoint → process group → final manifest → `diff.Compare` → persist; CLI `run` / `session list|show` / `diff`; commands take session ids |
| 5 | Receipt as trust surface: deterministic projection of persisted artifacts; undo availability; standing scope; no telemetry |
| 6 | `doctor`: readiness status READY / READY WITH WARNINGS / NOT READY; lock probe; bounded class walk |
| 7 | `recover`: repository-scoped latest/explicit recovery checkpoint; same restore engine; lineage on recovery manifests |

Destructive restore exists as of Batch 3. Session wrap exists as of Batch 4. Receipt exists as of Batch 5. Doctor exists as of Batch 6. Recover exists as of Batch 7.

Batch 7 completes the v0.1 local primitive: doctor → checkpoint → run → final manifest → diff → receipt → undo → verify → recover. Further work comes from real usage, not a new subsystem.

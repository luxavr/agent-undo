# 0004. Session state contract

## Status

Accepted.

## Context

Batch 4 wraps an agent as a parent process. The session is the product-level object. Checkpoints remain the restore source of truth. Numeric process exit codes cannot distinguish “the agent exited 3” from “Agent Undo failed internally.”

## Decision

### Identity

- Session id: 8 random bytes, uppercase hex, displayed and stored as `XXXXXXXX-XXXXXXXX`.
- Checkpoint id: `cp_…` (unchanged).
- A session **references** checkpoint and final-manifest ids. It does not replace them.
- Canonical CLI (`undo`, `verify`, `diff`, `session show`) takes a **session id**. Raw `cp_…` ids are rejected at the CLI. Tests may call `restore.Run` with a checkpoint id.

### States

```
CREATED → CHECKPOINTING → RUNNING → COMPLETED
CREATED → CHECKPOINTING → RUNNING → INTERRUPTED
CREATED → CHECKPOINTING → FAILED
CREATED → FAILED
```

In-progress states (`CREATED`, `CHECKPOINTING`, `RUNNING`) are persisted so a crash still leaves a record. They are not success.

### Outcomes (orthogonal to state)

| Outcome | Meaning |
|---|---|
| `CHILD_EXIT` | Child started and exited on its own |
| `INTERRUPTED` | Cancel/SIGINT/SIGTERM after the child started |
| `AGENT_UNDO_ERROR` | Lock, spawn, checkpoint, or other wrapper failure |
| `FINAL_SNAPSHOT_FAILED` | Child finished; final manifest could not be verified |
| `CHECKPOINT_FAILED` | Failed before a verified checkpoint existed |

### Process vs Agent Undo

Session record fields:

- `state`
- `outcome`
- `process.exit_code` (set when the child exited)
- `process.signal` (set when killed by signal)
- `agent_undo.error_code` (`3` on internal failure; otherwise unset/0)
- `agent_undo.error` (human text; no env dump)

**Exit status alone is not sufficient to distinguish an agent's exit code from an Agent Undo internal failure. Use the session record for attribution.**

Public wrapper exits:

| Situation | Exit |
|---|---|
| Child exited N and session `COMPLETED` | N |
| Interrupted after the agent started | 130 |
| Usage / invalid invocation | 1 |
| Agent Undo internal failure | 3 |

### Command matrix

| State | Checkpoint present | show | diff | verify | undo |
|---|---|---|---|---|---|
| CREATED | no | ✓ | ✗ | ✗ | ✗ |
| CHECKPOINTING | maybe | ✓ | ✗ | ✗ | ✗ |
| RUNNING | yes | ✓ | ✗ | ✗ | ✗ (lock held) |
| COMPLETED | yes | ✓ | ✓ | ✓ | ✓ |
| INTERRUPTED | yes | ✓ | ✓ | ✓ | ✓ |
| FAILED + `CHECKPOINT_FAILED` | no | ✓ | ✗ | ✗ | ✗ |
| FAILED + `FINAL_SNAPSHOT_FAILED` | yes | ✓ | ✗ (no final) | ✓ | ✓ |
| FAILED + `AGENT_UNDO_ERROR` after checkpoint | yes | ✓ | ✗ unless final exists | ✓ | ✓ |

`keep` is not a command. It means do nothing.

### Lock

One repository lock for the whole `run` (checkpoint → child → final manifest → persist). Contention prints exactly:

`Agent Undo session already active for this repository.`

`AGENT_UNDO_SESSION=<id>` may be set in the child environment as a marker. The lock is authoritative.

### Process group

The child is placed in its own Unix process group. stdout/stderr inherit (not stored). argv is executed directly (no `sh -c`).

On cancel: SIGINT → grace (default 2s) → SIGTERM → grace → SIGKILL, then wait. Escalation only if the child has not exited. Grandchildren are best-effort contained in the group. This is not kernel-level containment.

### Persistence

`~/.agent-undo/sessions/<session-id>/record.json` references manifest ids. Argv is stored as executed. Environment is not stored. stdout/stderr are not stored.

Command-line arguments are persisted as provided. Do not pass secrets directly as command-line arguments.

### Receipt

`run` and `session show` print the same deterministic projection of the session record plus referenced manifests. The receipt is not a source of truth. It does not scan the live workspace or git.

It reports checkpoint integrity (`VERIFIED` / `UNAVAILABLE`), undo availability, measured file counts, and a standing scope limitation. It does not print SUCCESS / FAILED (those belong to `verify` and `undo`). It does not pause for `[k] [i] [u]`. Next steps are printed as commands.

## Consequences

- Wrapper is orchestration only: `checkpoint.Create`, `process.Execute`, `checkpoint.Create` (`kind: final`), `diff.Compare`.
- Final snapshot after the child uses a context that is not canceled (interrupt still produces a final manifest).
- Restore APPLY is unchanged.
- Nested `agent-undo run` in the same repo fails on the lock, not on argv inspection.

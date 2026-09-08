# Architecture decisions (v0.1)

Implementation-oriented lock. Details live in the ADRs named below.

## Why Go?

Filesystem, signals, locks, and a single static binary. No Node in the runtime. [ADR 0001](adr/0001-go-for-core.md).

## Why local-first?

The product is Ctrl+Z for **this machine’s workspace**. Cloud rollback, SSO, and control planes cannot make restore correct. Store: `~/.agent-undo/` (override with `AGENT_UNDO_HOME` in tests). Optional `.git/agent-undo/` pointer must not be the only copy.

## Why wrapper-based v0.1?

`agent-undo run <agent command>` is a parent process: checkpoint, spawn, wait, receipt. Cursor/editor-native attachment is **not supported**. Do not fake it.

## What is a checkpoint?

A verified, content-addressed snapshot of supported local workspace state inside the repository boundary: regular files (per [ADR 0003](adr/0003-ignore-policy.md)), symlinks as links, empty dirs, and git metadata when captured ([ADR 0002](adr/0002-git-restore-policy.md)).

A checkpoint is real only after objects are hashed, written, and the manifest verifies against those objects.

## What is a session?

The product-level object for one wrapped run: identity, argv, lifecycle state, process result, and references to the session checkpoint and final manifest. Restore still loads the checkpoint. [ADR 0004](adr/0004-session-state.md).

## What is restorable?

The checkpoint. Files + captured git HEAD/branch overlay as specified in ADR 0002. Success = `verify` match, not write count.

## What is not restorable?

External side effects; skipped ignore classes; `.git/objects` as blobs; out-of-boundary writes; mtimes/xattrs/ACLs; Windows v0.1; anything not in the manifest. See [limitations.md](limitations.md).

## Trust model

- **After hash verify:** objects and manifests.
- **Untrusted:** agent argv, agent-written files, agent paths, repo contents, symlinks, user environment.
- **Hostile input:** any path used as a restore target, including session events.

Agent Undo is not a sandbox. Same uid as the caller.

## Failure model

Fail closed. Incomplete checkpoint → not restorable. Lock held → no overlapping `run`/`undo`. Missing captured SHA → no git restore. Verify mismatch → restore did not succeed. Do not silently fall back from git-aware restore to files-only.

Interrupted recovery-checkpoint → do not apply. Interrupted apply → recovery checkpoint exists; `recover` / `doctor` complete the story.

## Invariants

1. No destructive restore without a valid checkpoint.
2. No destructive restore without a verified recovery checkpoint.
3. No writes outside the repository boundary.
4. Never trust agent-provided paths; canonicalize against the boundary.
5. Symlinks are security-sensitive; store as links; do not follow out.
6. Success is owned by verify.
7. Restore failures are reported; never discarded.
8. Never silently rewrite git history; never force-push, delete branches, or prune session commits.
9. Receipts list only captured facts.
10. One exclusive lock per repository boundary for `run` and `undo`.
11. Process result and Agent Undo result are separate; exit status is not attribution.
12. Canonical CLI targets are session ids.

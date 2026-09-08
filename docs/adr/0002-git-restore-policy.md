# 0002. Git restore policy (Ctrl+Z the repo)

## Status

Accepted.

## Context

A checkpoint captures filesystem state and, when present, git metadata (HEAD, branch, dirty working tree). The agent (or the human) may commit, amend, switch branches, or otherwise move HEAD during a protected session.

`git reset --hard <captured HEAD>` restores **HEAD**, not the pre-session working tree. Pre-session dirty tracked files, untracked files, and captured ignored files live in the checkpoint, not in that SHA.

Silent history rewrite is forbidden. Force-push, branch deletion, and pruning unreachable objects are forbidden.

A git repository is **not** required. See also [0003-ignore-policy.md](0003-ignore-policy.md).

## Decision

**The checkpoint is the source of truth. Git is one dimension of state, not the whole state.**

Policy name: **Ctrl+Z the repo.**

### Capture (read-only)

When `<boundary>/.git` exists, the checkpoint manifest records:

- `git.captured = true`
- HEAD SHA
- branch name, or detached HEAD
- enough metadata to describe pre-session dirtiness (tracked dirty / untracked / captured-ignored)

The filesystem walk **does not** copy `.git/` (including `objects/`). Git object identity is the SHA recorded in the manifest. Blob restore of the working tree is the checkpoint overlay.

When `<boundary>/.git` is absent: `git.captured = false`. Receipt must say `Git: not captured`. No git operations on restore.

### Undo sequence (destructive restore, git-aware)

Only after [architecture.md](../architecture.md) restore pipeline stages:

1. Exclusive repo lock.
2. Validate repository identity (canonical root matches manifest) and, if `git.captured`, that captured HEAD still exists in the object database.
3. Verified **recovery checkpoint** of current post-agent filesystem + git metadata (HEAD, branch). This SHA is how `recover` puts the post-agent state back.
4. Show an explicit restore plan (TTY confirm unless `--yes`).
5. Move HEAD back to the captured SHA when HEAD (or the checked-out branch) differs from capture. Overlay the checkpoint **after** any HEAD move.
6. Overlay checkpointed pre-session working tree, index intent, untracked files, and captured-ignored files. The overlay is authoritative for dirty pre-session state.
7. Verify the resulting workspace (and git HEAD, when captured) against the checkpoint. Success is verify, not write count.
8. Never force-push. Never delete branches. Never prune unreachable git objects (`gc --prune`, `repack -d` that drops session commits, etc.).

Session commits are **not** deleted. After undo they are unreachable from the restored branch and remain addressable by the recovery checkpoint’s recorded HEAD.

### Restore plan (must show before apply)

```
CAPTURED   HEAD / branch before session
SESSION    HEAD / branch after session
PROPOSED   HEAD / branch after undo
```

Plus:

- commits created during the session (reachable from session HEAD, not from captured HEAD)
- branch changes
- dirty changes that existed before the session (from the checkpoint, not from `reset --hard`)
- what will become unreachable from the restored branch
- how `recover` restores the post-agent state (recovery checkpoint id + recorded HEAD)

### Fail closed

Refuse git restoration (do not apply) when:

- git restoration is requested (`git.captured`) but `.git` is missing
- captured SHA is missing from the manifest or from the object database
- canonical repo root does not match the manifest
- branch/ref state is ambiguous (unborn HEAD, unexpected worktree, cannot resolve captured ref)
- verification mismatches after apply

Files-only restore is **not** a silent fallback when `git.captured` is true.

When `git.captured` is false, restore is filesystem-only. Do not invent git state.

### What this policy is not

- Not `git checkout` of a stash.
- Not rewriting remotes or un-pushing.
- Not deleting the agent’s branch.
- Not restoring `.git/objects` by copying blobs out of the object store into git.

## Consequences

- Restore apply (Batch 3+) must implement HEAD move **then** overlay, never overlay-only when `git.captured` and HEAD moved, never `reset --hard` without overlay.
- Recovery checkpoint is P0 data. `recover` restores a `kind: recovery` checkpoint using the same engine; a new recovery checkpoint is taken before APPLY.
- `adapters/git` may read from Batch 1. It must not write until restore apply exists and this ADR is followed.
- Receipt reports captured HEAD before/after when git was captured; otherwise `Git: not captured`.

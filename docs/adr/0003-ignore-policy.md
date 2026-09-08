# 0003. Ignore / capture policy

## Status

Accepted.

## Context

Gitignored does not mean irrelevant. Skipping `.env.local` makes the “agent deleted my env” case unrestorable. Snapshotting `node_modules` or `.git/objects` makes checkpoints huge and slow.

The checkpoint is the source of truth for **supported** local files inside the repository boundary. Capture must be explicit.

## Decision

### Include (when inside the repository boundary)

| Class | Size cap |
|---|---|
| Tracked files (git index) | none |
| Untracked files | 1 MiB |
| Gitignored **small user-owned** files | 1 MiB |
| `.env*` (regular files) | 1 MiB |
| Other small local config files that are untracked or ignored | 1 MiB |

Tracked means `git ls-files --cached` (or equivalent index read). A 20 MiB tracked fixture is captured. A 20 MiB gitignored tarball is not.

When git is not present, every regular file is treated as untracked: the 1 MiB cap applies, plus directory exclusion classes below.

### Exclude by default (directory skip: do not descend)

Report **classes** in doctor and in the manifest exclusion summary, not every path.

| Class | Match (path component or well-known dir) |
|---|---|
| `git.dir` | `.git/` (objects included; git metadata is [0002](0002-git-restore-policy.md), not a blob copy) |
| `node_modules` | `node_modules` |
| `vendor` | `vendor` |
| `dist` | `dist` |
| `build` | `build` |
| `target` | `target` |
| `next` | `.next` |
| `pycache` | `__pycache__` |
| `venv` | `.venv`, `venv` |

Also exclude, as file/object classes:

| Class | Rule |
|---|---|
| `oversize` | Untracked or ignored regular file > 1 MiB (1048576 bytes) |
| `socket` | Unix sockets |
| `device` | Device files |
| `nonregular` | FIFOs, other non-regular non-symlink types |
| `escape` | Path that canonicalizes outside the repository boundary |
| `symlink.external` | Symlink whose **target string** would restore outside the boundary — still stored as a link; target bytes are never copied in. Doctor lists the class. |

Names are matched as path components (a `src/node_modules/pkg` tree is skipped). They are case-sensitive on macOS/Linux as the filesystem presents them.

### Symlinks (capture)

Store as a link: relative path, mode, target **string**. Do not follow the link during traverse. Do not copy external target contents. See [security-model.md](../security-model.md).

### Empty directories

Capture empty directories that are not an excluded class, so restore can recreate them.

### Doctor

Doctor prints exclusion **classes** with counts (e.g. `node_modules: skipped 1 tree`). It does not dump every excluded path.

## Consequences

- `internal/ignore` is the single classifier. Checkpoint traversal must not invent a second skip list.
- Changing the cap or skip names is an ADR change, not a silent tweak in restore.
- Go `vendor/` trees are skipped in v0.1 (limitation, not a bug).
- `env/` as a generic folder name is **not** skipped; only `.venv` and `venv`.

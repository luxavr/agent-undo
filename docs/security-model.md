# Security model

Agent Undo is a **data-safety** tool, not a sandbox. It runs as the user. A hostile agent with the user’s privileges can destroy data this tool does not hold, including `$HOME/.agent-undo`.

Never market this as isolation.

## Assets

- Repository working tree inside the boundary
- Checkpoints and manifests
- Recovery checkpoints
- Repository lock files

## Trust boundaries

- **Trusted at rest only after hash verify:** objects and manifests.
- **Untrusted:** agent argv, agent-written files, agent-provided paths, repo contents, symlinks, inherited environment.
- **Never trusted as restore targets without canonicalization:** paths from events, manifests that fail hash, user argv paths.

## Path and symlink rules

- One package: `internal/security`. Adapters do not roll their own `Rel` checks.
- Every captured path must resolve inside the repository boundary.
- Traverse with `lstat`. Do not follow symlinks out of the boundary.
- Store symlinks as `{mode, path, target string}`. Do not copy external target bytes.
- Restore: canonicalize the destination; refuse writes that escape; restore dangling **in-repo** links as links.
- Out-of-repo symlink targets: class `symlink.external` in doctor; no inlining.

## Threats and MVP posture

| ID | Threat | Defense | Residual |
|---|---|---|---|
| T1 | Path traversal | Canonicalize; require inside boundary after clean | Bugs in the checker |
| T2 | Symlink escape | Detect; store as link; do not follow out; refuse escaping writes | In-repo links to surprising in-repo places |
| T3 | Corrupted checkpoint | SHA-256 objects; verify before apply | Bitrot after verify, before apply (lock + re-verify) |
| T4 | Agent deletes checkpoint | Default store under `~/.agent-undo/` | Agent can still delete `$HOME` |
| T5 | Concurrent restore/run | One exclusive lock; fail closed | Held lock: `doctor` reports pid, no steal. Leftover lock files are not held. |
| T6 | Partial write | Temp + rename; no success before verify | FS without atomic rename (not a v0.1 Windows target) |
| T7 | Interrupted restore | Recovery checkpoint first; crash state | Kill during recovery checkpoint: fail closed, do not apply |
| T8 | Malicious repo contents | Do not execute repo hooks/scripts to checkpoint; names are bytes | User may `run` a hostile binary |
| T9 | Privilege escalation via spawn | No sudo, no setuid; same uid; child process group is best-effort | User already had those privileges; grandchildren may escape the group |
| T10 | Secrets on argv | Document: do not pass secrets as arguments; persist argv as given; persist no env | Args remain in `record.json` |

## Destructive operation rules

- Explicit paths only. No glob apply.
- Overlay may `RemoveAll` a path only after `JoinRel` has proven it is inside the repository boundary. It unlinks symlinks; it does not follow them.
- Never `rm -rf` a user-supplied string or raw argv path.
- Never write outside the repository boundary.
- Never overwrite checkpoint bytes in place.
- Locks around `run` and `undo`.
- Git writes only per [ADR 0002](adr/0002-git-restore-policy.md): no force-push, no branch delete, no prune.

## What we tell the user

Doctor and the README: not a kernel sandbox; ignore policy omits classes; external side effects are not undone; success means verify passed against a checkpoint we still have; v0.1 is wrapper-based (no Cursor-native attach).

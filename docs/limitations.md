# Limitations

Honesty is a feature. The public one-pager is [v0.1-contract.md](v0.1-contract.md). This annex is the detailed list. Do not fork a third copy into the README.

Agent Undo restores **supported local workspace state** recorded in a checkpoint. It does not reverse the rest of the world.

## v0.1 attachment

Cursor/editor-native attachment is not supported. v0.1 is wrapper-based: `agent-undo run <agent command>`.

## Platforms

Supported: macOS, Linux. Release assets: `darwin`/`linux` × `amd64`/`arm64`. CI executes tests on `ubuntu-latest` and `macos-latest`. Cross-compiled targets are built, not executed.

Windows is unsupported in v0.1 and is not a launch blocker.

## Will not reverse (MVP, and likely ever for these)

- Email, Slack, issues, tweets
- Cloud APIs, billed usage, purchases
- Databases except files that sat in the workspace and were checkpointed
- Remote git (`push`, deleted remotes, rewritten remotes). Undo never force-pushes.
- Kubernetes, AWS, DNS, SaaS
- Browser / editor UI state
- Anything after the process left the machine

## Will not reverse in v0.1 (by policy)

- `node_modules`, `vendor`, `dist`, `build`, `target`, `.next`, `__pycache__`, `.venv`/`venv` trees
- `.git/objects` (git identity is HEAD SHA + overlay, not a copied object DB)
- Untracked/ignored files over 1 MiB
- Sockets, devices, FIFOs
- File mtimes, xattrs, BSD flags, ACLs, resource forks
- Writes outside the repository boundary
- Staging / git index fidelity (HEAD + filesystem bytes only)
- Go `vendor/` (skipped as an exclusion class)

## Git

When git was not captured: `Git: not captured`. Filesystem-only restore.

When git was captured: undo moves HEAD back to the captured SHA if needed, then overlays the checkpoint. Session commits are not deleted; they become unreachable from the restored branch and remain named by the recovery checkpoint. See [ADR 0002](adr/0002-git-restore-policy.md).

**Git index / staging is not restored.** v0.1 restores **HEAD + checkpointed filesystem bytes**. Pre-session staged changes may appear as unstaged after undo. Verification tests that contract, not `git diff --cached` fidelity.

## Will refuse rather than guess

- Corrupt or incomplete checkpoint
- Repo root mismatch
- Lock held (`run` or `undo` already in progress)
- `git.captured` but missing `.git` or missing captured SHA
- Ambiguous branch/ref state
- Path that escapes the repository boundary
- Unverified “success”
- `run` when the cleaned working directory is the user home directory (exit 1), or when that identity cannot be proven (exit 3). `doctor` from proven `$HOME` warns; it does not refuse inspection and does not label unproven identity as home.
- `recover --yes` without an explicit recovery checkpoint id. Target selection may not be implicit.

Doctor exists to surface these before the user trusts a session. It reports readiness; it does not repair. Standing limitations are always listed and do not by themselves make the install `NOT READY`.

## Session wrapper

- **Undo requires a started process.** LookPath failure and `cmd.Start` failure leave a session checkpoint (verify/show still work) but `Undo: UNAVAILABLE`. This is eligibility, not a promise that undo is safe after later work on a started session.
- **Exit status alone is not attribution.** A child may exit 3 and the session is `COMPLETED` / `CHILD_EXIT`. Agent Undo internal failure also exits 3 with `FAILED` / `AGENT_UNDO_ERROR`. Use `session show`.
- Command-line arguments are persisted as provided. Do not pass secrets directly as command-line arguments. There is no redaction.
- Environment variables, stdout, and stderr are not stored in the session record.
- The child is placed in its own process group. On cancel: SIGINT, then SIGTERM, then SIGKILL, each after a grace period. Grandchildren are best-effort contained in that group. This is not kernel-level sandboxing.
- `keep` is not a CLI command. The receipt prints next-step commands instead of an action loop.

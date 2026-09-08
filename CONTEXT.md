# Agent Undo

Local-first safety layer around an AI coding agent session. Glossary only — no implementation.

## Language

**Agent Undo**:
The product. A CLI that checkpoints a workspace, wraps an agent session, and can restore the pre-session local state.
_Avoid_: AgentSafe, AgentGuard, AI governance platform, control plane (for the MVP)

**Session**:
One protected agent run, bounded by checkpoint creation and session completion (or interruption).
_Avoid_: job, trace, execution (except when talking about the child process)

**Checkpoint**:
A verified, content-addressed snapshot of supported local workspace state. Source of truth for restore. Git is one dimension of that state, not the whole state.
_Avoid_: backup, dump, Time Machine, stash (those are other tools)

**Recovery checkpoint**:
A checkpoint of post-agent (current) state taken immediately before a destructive restore, so undo itself can be undone. Optional `source` lineage records whether undo or recover created it.
_Avoid_: backup of the backup (except in user-facing copy)

**Manifest**:
The versioned record that names a checkpoint: id, repo root, git capture (or `captured: false`), file paths, hashes, modes, sizes, exclusion classes.
_Avoid_: index, catalog, inventory

**Object store**:
Content-addressed storage of file bytes, keyed by hash, living under `~/.agent-undo/` unless `AGENT_UNDO_HOME` is set.
_Avoid_: cache, blobstore as a product name

**Restore**:
The operation that returns the workspace to a checkpointed state, after a recovery checkpoint, then verifies. Git HEAD move (when captured) then checkpoint overlay.
_Avoid_: rollback (reserved for future infra), revert (git-specific), reset (git-specific unless the git adapter is doing that)

**Verify**:
Recompute current state and prove it matches the target checkpoint. Success is not claimed before this.
_Avoid_: checksum pass (too narrow), “it compiled”

**Receipt**:
The human-readable session summary printed by `run` and `session show`: what started, agent result, measured file counts, git refs or not captured, checkpoint integrity, undo availability, standing restore scope, next commands. Only facts from persisted artifacts. Not a live view of the workspace.
_Avoid_: report, log dump, dashboard, fake command/risk telemetry, SUCCESS/FAILED for a post-agent workspace (that is `verify`)

**session show**:
CLI for receipt and session metadata.
_Avoid_: inspect (not a command)

**diff**:
CLI for file/state delta versus the session checkpoint.
_Avoid_: inspect as a synonym

**recover**:
CLI that restores a previously created recovery checkpoint (`kind: recovery` only) for the current repository. Optional id; default is latest valid. Same restore engine as undo.
_Avoid_: repair, crash-complete as a separate mechanism

**Keep**:
Accept the post-session state. Checkpoint remains available until cleaned.
_Avoid_: commit, save (git/OS meanings)

**Doctor**:
A readiness command: can this environment keep Agent Undo's promises? Status READY / READY WITH WARNINGS / NOT READY. Reports exclusion **classes**, lock state, standing limitations. Does not repair.
_Avoid_: health check as the user-facing name, security scanner

**Workspace state**:
The supported local files and git metadata inside the repository boundary. The only thing MVP restore claims to reverse.
_Avoid_: system state, environment, universe

**Repository boundary**:
The canonicalized workspace root. Restore must not write outside it.
_Avoid_: project, folder (too loose)

**Lock**:
A repository-scoped exclusive lock for `run`, `undo`, and `recover`. Fail closed. No silent steal.
_Avoid_: mutex as user copy

**Adapter**:
A narrow integration over git, filesystem, shell, or a named agent. Core packages must not depend on a specific agent.
_Avoid_: plugin, integration marketplace

**External side effect**:
Anything outside supported local workspace state: APIs, emails, remote git, cloud, databases, purchases. Not reversible in MVP.
_Avoid_: “not supported yet” as if it were a missing checkbox for the same primitive

# PRD v2 — deep analysis

Source: [prd-v2.txt](prd-v2.txt), compared to [prd-v1.txt](prd-v1.txt).

**v0.1 is locked.** Git, ignore, attach, CLI, and restore contracts live in [architecture.md](architecture.md) and ADRs 0001–0003. [open-questions.md](open-questions.md) is a pointer table, not an open debate. This file remains the PRD reading notes.

---

Agent Undo is a **reversible execution layer** for local AI coding sessions. The product is not “agent security,” not a control plane, and not git. It is Ctrl+Z for the workspace the agent just mutated.

The PRD is unusually complete as a handoff: product, architecture, safety, tests, launch, and a Cursor coding loop. It is also internally inconsistent in a few places that will ship the wrong CLI, the wrong Batch 0, or an unsafe restore if followed literally. Those are called out below as **must-resolve** vs **judgment**.

## 1. What is actually being built

One loop:

```
checkpoint(workspace) → agent runs → receipt → keep | inspect | undo
```

Undo is not `git checkout`. Git often does not see the damage: uncommitted edits, untracked files, ignored files, dependency trees, files the agent created then deleted. The primitive is **session-scoped workspace restore**, with git metadata as one captured facet.

MVP success is five user-visible facts:

1. A session can be started under protection.
2. The agent can mutate the repo.
3. The user can see what happened.
4. Undo returns supported local state to the pre-session checkpoint.
5. Verify can prove it, or failure is explicit — including a recovery checkpoint so undo can itself be undone.

If (4) or (5) is weak, the product is a toy. The rest of the 64-section PRD is distribution, tone, and a company map that must not enter the MVP.

## 2. What v2 actually decided

v2 is not a rewrite of the product. It is a **language lock** plus a sharper engineering brief.

| Decision | v1 | v2 |
|---|---|---|
| Language | TypeScript **or** Go | **Go only** for core/CLI |
| Node in core | Possible | Forbidden |
| Layout | `src/cli`, `src/core`, … | `cmd/`, `internal/`, `adapters/` |
| Tooling | implied | gofmt, vet, race, fuzz, cross-platform CI |
| Cursor brief | 10 rules | Ordered priorities, restore contract, security, test bar |
| Batch 0 | “TS config OR Go modules”; npm acceptance | Same leftover text — **stale** |

Keep Go. Treat every remaining “OR TypeScript” / `npm install` line as a merge artifact. Details: [v1-vs-v2.md](v1-vs-v2.md).

## 3. The real product risk is not restore math

Checkpoint → restore is a known class of problem (content-addressed snapshots, like git objects without the SCM). The hard product problems are **boundary** and **how the tool attaches to the agent**.

### 3.1 Wrap vs inhabit

The CLI is designed as:

```bash
agent-undo run claude
agent-undo run -- npm run agent
```

That is a **parent process wrap**. It works for Claude Code CLI, Codex-style CLIs, and any agent you actually spawn.

It does **not** match how Cursor is used. The Cursor agent is already inside the editor. The developer will not typically relaunch Cursor under `agent-undo run`. If “especially Cursor” stays in the primary user list, MVP needs a second attach mode or the README must tell the truth: **v0.1 protects agents you launch through Agent Undo**. IDE-sidecar / folder-watch “protect this session I already started” is a later adapter.

This does not kill the launch. Claude Code + terminal agents are enough for a 30-second demo. It does kill a README that implies Cursor is a first-class wrap on day one.

### 3.2 Snapshot is enough for undo; events are not required for undo

Two different products share one receipt:

| Capability | Needs live events? | MVP? |
|---|---|---|
| Undo workspace | No. Before/after snapshot is sufficient | MUST |
| Inspect file diff | No. Diff two manifests | MUST |
| “6 commands observed” | Yes, unless you only count the wrapped child | SHOULD |
| “1 suspicious command” | Yes, plus a risk heuristic | later (v0.5) |
| Package changes | Maybe: diff lockfiles in the snapshot | SHOULD, via diff not a package registry |

The mock receipt overpromises command and risk observability. Ship a receipt that reports **what was actually captured**. Doctor already models this honesty (`ignored files not captured`, `external side effects cannot be reversed`). The session receipt should do the same: if commands were not recorded, do not print a command count.

Recommended MVP inspect surface: files created/modified/deleted/renamed, git HEAD/status delta, lockfile/package manifest deltas. Commands only if the process adapter actually recorded them.

### 3.3 Git policy is unspecified and dangerous

The PRD captures `gitHead` and promises “filesystem/git state” and “working tree restored.” It never says what happens if the agent **committed**, **amended**, **checked out a branch**, or **force-reset**.

Four policies, pick one before Batch 3:

| Policy | Behavior | Risk |
|---|---|---|
| Files only | Rewrite files; leave HEAD/index/branch as the agent left them | “Undo” that still has the agent’s commits |
| Working tree + index, keep HEAD | Restore files and index; do not move HEAD | Commits remain; working tree matches checkpoint |
| Reset to captured HEAD | `git reset --hard <captured>` plus untracked/ignored restore | Drops session commits; matches “rewind the repo” intuition |
| Refuse if HEAD moved | Fail closed, tell the user to decide | Safest, worst demo |

Recommendation: **default = reset to captured HEAD + restore untracked/ignored that were in the checkpoint**, after the recovery checkpoint, with the restore **plan** listing `git HEAD 91eaf12 → (agent) abcdef0` so the user sees history will move. Document that session commits are discarded by undo. That matches Ctrl+Z. Make it explicit in [limitations.md](limitations.md).

Until that call is written into an ADR, do not implement restore.

### 3.4 Created-file “ownership”

Restore step 9: remove files created after checkpoint **where ownership can be established**.

Without a watcher, “created after” means “present now, absent from checkpoint.” That will delete files the **human** wrote during the session, and files a parallel process created.

Options:

1. **Treat the session as exclusive.** Document: while a session is open, don’t touch the tree yourself (or those edits are in the undo blast radius). Simplest. Matches wrap UX.
2. **Interactive plan** lists every delete. Default in interactive TTY; `--yes` for demo scripts.
3. **Watcher ownership.** Only delete paths that appear in `file.created` events. Incomplete (misses grandchild writes) but less surprising.

Recommendation: (1) + (2). Exclusive session + always show the delete list. Watcher ownership is Batch 4+, not Batch 3.

### 3.5 Ignored files: the .env vs node_modules split

MUST says “ignored files where safely possible.” Capturing `node_modules` makes checkpoints huge and slow. Not capturing `.env.local` means the demo “agent deleted my env” fails.

Default should be:

- Capture gitignored **small, user-owned** files (dotenvs, local config).
- Skip well-known bulky trees: `node_modules`, `.git/objects`, build dirs, virtualenvs.
- Doctor warns when ignore policy omitted something the user might care about.
- Never put the object store inside the repo by default (agent can `rm -rf` it). `~/.agent-undo/` is correct. `.git/agent-undo/` is optional and must not be the only copy.

## 4. Architecture that should survive contact with the code

Lock the packages in [architecture.md](architecture.md). The important contracts:

**Store.** Content-addressed objects + manifests + session logs + locks under `~/.agent-undo/`. Integrity hash on every object. Treat the store as untrusted on read (T3, T4, T8).

**Checkpoint.** Traverse within the repository boundary, apply ignore policy, hash, write objects atomically, write manifest, fsync, then consider the checkpoint real. Never call a checkpoint complete if verification of the manifest against objects fails.

**Restore.** The PRD algorithm is right. Collapse Batch 3 and the *data* part of Batch 6: a restore that cannot take a recovery checkpoint must abort. `agent-undo recover` (the command) can wait for Batch 6; the **checkpoint of post-state** cannot.

**Verify.** Independent of restore. `restore` calls `verify`. `agent-undo verify` can run alone. Success string is owned by verify, not by the writer.

**Security.** Path canonicalization and symlink policy are a single package (`internal/security`). Every write path goes through it. Adapters do not roll their own `filepath.Rel` checks.

**Process.** `agent-undo run` spawns the child with a context, forwards signals, records exit, and cannot leave a session “running” after the parent dies without a crash record. Interrupted checkpoint/restore is a first-class state, not a missing file.

## 5. Contradictions inside v2 (must-resolve)

These are not taste. Following both sides produces the wrong program.

### C1. Batch 0 still describes a Node project

PRD §48 Batch 0: “TS config OR Go modules” and acceptance “npm/pnpm install works.”

§9 and §49: Go only, no Node in the core.

**Resolve:** Batch 0 acceptance is `go test ./...` green, CLI `agent-undo --help` boots, CI runs gofmt/vet/test. See [adr/0001-go-for-core.md](adr/0001-go-for-core.md).

### C2. Two CLI surfaces

§9: `init`, `status`, `sessions`, `inspect`, `clean`.

§15 / §49: `session list|show`, `diff`, `recover`, `doctor`. No `init`.

**Resolve:** Canonical = §15. `init` is unnecessary if `run` auto-detects a repo (Principle 8). `clean` can wait. `status` is `session list` of the in-progress session.

### C3. Recovery is both P0-critical and P1/Batch 6

Restore step 7 and the safety contract: never destructive restore without a recovery checkpoint.

Executive priority: Recovery is P1. Batch 6 is after CLI chrome.

**Resolve:** Split the word.

- **Recovery checkpoint** (data): part of Batch 3. P0. No restore without it.
- **Recover command / “undo has an undo” UX**: Batch 6. P1.

### C4. Receipt mock vs MVP MAY/SHOULD

The ASCII receipt includes commands observed, packages changed, suspicious commands. Those are SHOULD/later.

**Resolve:** Receipt schema is computed from captured facts. Placeholders and jokes are fine; fake telemetry is not.

### C5. `inspect` vs `session show` vs `diff`

All three appear. One user-facing inspect path: `session show` (receipt + summary), `diff` (structural file diff). Drop `inspect` as a verb or make it an alias of `session show`.

## 6. Threats that should shape Batch 1–3

Full model: [security-model.md](security-model.md). The ones that change design now:

| ID | Threat | Design consequence |
|---|---|---|
| T1/T2 | Path traversal / symlink escape | Central canonicalize + refuse write outside repo; do not follow symlinks out; decide whether to snapshot symlink as link or target |
| T3 | Corrupted checkpoint | Hash every object; verify manifest before apply; refuse |
| T4 | Agent deletes checkpoint | Default store **outside** the repo. Agent with `$HOME` access can still win; document that |
| T5 | Concurrent restore | Repo lock, fail closed |
| T6/T7 | Partial / interrupted write | Atomic replace (write temp + rename), recovery checkpoint, crash-complete on next command (`doctor` / `recover`) |
| T8 | Malicious repo contents | Do not execute repo scripts during checkpoint. Manifest is data. |
| T9 | Spawned command privilege | `run` should not elevate. Pass through user uid. Do not wrap in sudo. |

What MVP does **not** protect: a hostile agent with the same privileges as the user, kernel bugs, anything off-box. Doctor must say this.

## 7. Test bar (what “done” means)

The property that matters:

```
verify(restore(checkpoint(A), B)) ≈ A
```

with documented exceptions (mtime, atime, ctime, maybe xattrs, maybe Unix socket files, git objects inside `.git` if not modeled).

Minimum fixture matrix for Batch 1–3:

- 1 file, 100 files, nested, empty, unicode, binary
- create / modify / delete / rename
- tracked vs untracked
- ignored small file vs skipped bulky dir
- symlink (file, dir, dangling, escape attempt)
- permission denied mid-restore
- SIGINT during checkpoint and during restore
- concurrent second `undo`
- malformed and truncated manifests

Chaos (Batch 7) can come after the happy path is proven, but interruption tests for checkpoint and restore are not optional if we claim crash recovery.

## 8. Scope that must stay cut

Do not let the company map leak into v0.1:

- policy.yaml, allow/ask/deny
- risk engine prompts
- cloud, SSO, RBAC, billing
- database/AWS/K8s rollback
- “Datadog + Cloudflare + Git for AI agents” as public copy
- 50 agent integrations — one wrap (`run --`) is the adapter

The cut rule in the PRD is correct: if it does not improve **reversibility, visibility, or trust**, it is out.

## 9. Launch reading (so engineering doesn’t optimize the wrong artifact)

The wedge is the **GitHub repo + 30s catastrophic demo**, not a landing page. Engineering P0 is: install, checkpoint, restore, verify, honest limitations, no account.

Viral property that is also architecture: **“Undo has an undo.”** That is why the recovery checkpoint is not polish.

The north-star metric (protected sessions) is downstream of trust. One botched restore on Hacker News ends the launch. Bias every trade toward “we refused to restore” over “we restored wrong.”

## 10. Recommended implementation order (reconciled)

0. Bootstrap: Go module, `cmd/agent-undo`, CI, help text, README skeleton (this repo).
1. Snapshot engine + object store + manifest + path security.
2. Diff two checkpoints.
3. Restore + recovery checkpoint + verify. Git policy ADR required before this batch.
4. `run` wraps a child process, session id, receipt from snapshot diff.
5. CLI chrome: colors, interactive keep/inspect/undo.
6. `recover` command and crash-complete.
7. Chaos tests.
8. Docs, doctor, install, limitations honesty.
9–10. Demo and launch — not code features.

## 11. Judgment: what is strong vs what is theater

**Strong:** local-first, fail closed, recovery checkpoint, honest limitations, content-addressed store, restore-after-verify, doctor as anti-false-confidence, wrap-shaped CLI, Go, ruthless MVP cut.

**Theater if built early:** joke density in the receipt, risk scores, package “observation” beyond lockfile diff, named adapters (`agent-undo claude`) before `run --` works, team/cloud sections occupying engineering attention.

**Missing until decided:** git HEAD policy, ignore policy, IDE attach, created-file ownership, Windows in v0.1 or not (contributor bait later is fine; CI matrix is not free).

Those live in [open-questions.md](open-questions.md).

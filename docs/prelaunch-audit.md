# Pre-launch adversarial audit

Status: **PASS WITH FIXES**

Feature freeze holds. This audit tried to break the existing promise. It did not add capabilities.

**BLOCKER** = restore/security/data-safety defect, or contract/README/binary mismatch, that must be resolved before `v0.1.0`.
**OBSERVATION** = UX, wording, coverage gap, or documented limitation. Must not be silently reclassified as a blocker later without evidence.

Clean-machine journeys A–D are **not** claimed here. A human still owns those before tag.

## Critical findings

- **Fixed (restore-correctness).** If a checkpointed file path was replaced by a directory whose children were **not** in the live scan (for example an oversize untracked file), overlay used `os.Remove` on a non-empty directory and restore returned `INCOMPLETE`. Overlay now `RemoveAll`s only after `JoinRel` has proven the path is inside the repository boundary. Symlinks are unlinked; targets are not followed. Tests: `TestFileReplacedByDirWithUncapturedChild`, `TestOverlayUnlinksSymlinkWithoutFollowing`.
- No remaining critical findings known.

## Data safety

| Probe | Result |
|---|---|
| Recovery checkpoint before APPLY | Covered. Cancel after recovery, before overlay: recovery id exists, live bytes unchanged. |
| Abort / `--yes` missing | No mutation. |
| Incomplete objects | Fail closed. CLI explicit recover: `NOT READY TO RECOVER`, exit 3. |
| Corrupt object hash | Store get fails closed. |
| Nested `run` | Exact lock message; no steal. |
| Concurrent restore while lock held | Fail closed. |
| Selected recover target invalid after lock | Fail closed; does not switch to a newer recovery. |

No open **BLOCKER**.

## Restore correctness

Covered in tests: non-git create/delete/modify/rename leftover; in-repo symlink restore; git dirty + untracked + captured `.env.local`; session commit + branch move (session commit remains in the object DB); rewritten/orphan history; missing captured SHA; file→dir with captured children; agent-created directory removal; recover A→B→C chain; lineage optional; old recovery manifests without `source`.

| Probe | Result |
|---|---|
| Binary files | Bytes + SHA-256; no encoding path. No dedicated “binary” test; treated as **OBSERVATION**. |
| Large tracked files | Captured (ADR 0003). Tested. |
| Large untracked files | Skipped as `oversize`. Tested. |
| Excluded trees | `node_modules` and class peers skipped. Tested. |
| Type replacement + uncaptured child | Was a blocker; **fixed** this session. |
| Git index / staging | Not restored, by contract. **OBSERVATION**: no dedicated `git diff --cached` assertion; doctor note exists. |
| Detached HEAD restore | Implemented (`MoveHEAD` detach). **OBSERVATION**: no dedicated test. |
| Deleted captured branch | Validate refuses. **OBSERVATION**: no dedicated test. |
| Staged-only pre-session changes | Become unstaged after undo (HEAD + bytes). Documented. |
| Concurrent human writes during APPLY | Lock is cooperative among Agent Undo processes, not a kernel freeze of the tree. **OBSERVATION**. |

## Security boundary

| Probe | Result |
|---|---|
| Path `..` / absolute escape | `security.Rel` / `JoinRel` reject. Fuzzed. |
| Symlink node inside, target outside | Stored as link; `symlink.external`; bytes not inlined. |
| Overlay must not follow external symlink | Tested after the RemoveAll fix. |
| NUL in path | Rejected. |
| Restore writes | Only through `Boundary`. |
| Argv injection | Literal argv; no shell. Tested. |
| Env persistence | Session record has no env. Tested. |

No open **BLOCKER**. Residual: a hostile agent with the user’s uid can still delete `$HOME/.agent-undo`. Not a sandbox. Documented.

## Sessions

| Probe | Result |
|---|---|
| Child exit 0 / non-zero | Preserved; receipt attribution via record. |
| Missing command | Start error; internal path. |
| SIGINT cancel | Wrap interrupt + process escalation tests. |
| SIGTERM / SIGKILL of child | Escalation SIGINT→TERM→KILL on cancel. |
| Child hang | Cancel interrupts `sleep`. |
| Grandchildren | Best-effort process group. **OBSERVATION**: not kernel sandbox. |
| Nested Agent Undo | Lock. |
| Concurrent `run` | Lock. |
| SIGKILL of Agent Undo during APPLY | Interrupt tests at pipeline points; kill -9 of the parent is still crash/incomplete + recovery checkpoint if that stage finished. Documented T7. |

## Recovery

| Probe | Result |
|---|---|
| Repeated undo / recover | Chain tests; older recoveries remain. |
| Recover after verify failure | Hint `agent-undo recover <new-id>`; no auto-recover. |
| Corrupt / incomplete recovery | Fail closed. |
| Old-format manifest (no `source`) | Loads; lineage ignored by APPLY/VERIFY. |
| No eligible recovery | Exact stderr, exit 1, no lock required for lookup. |
| Session id / non-recovery `cp_` | Exit 1. |

## CLI behavior

| Probe | Result |
|---|---|
| Help / unknown / extra args | Usage exit 1. |
| `undo` success copy | Prints `SUCCESS` plus recovery id. `recover` prints a longer checkmark block. **OBSERVATION**: inconsistent UX, not a correctness defect. README shows the real `undo` lines. |
| Receipt vs verify vocabulary | Receipt does not say SUCCESS/FAILED. |
| Doctor | READY / WARNINGS / NOT READY; leftover `.lock` file is not held. |

## Documentation

| Item | Result |
|---|---|
| Public contract | `docs/v0.1-contract.md` |
| Annex | `docs/limitations.md` |
| README | Product-shaped; links contract + limitations; does not invent a third spec |
| Help “yet” | Removed so freeze does not imply a scheduled Cursor attach |
| Clean-machine journeys | Happy-path T1–T4 re-measured on darwin/arm64 against the `v0.1.0` GitHub Release. Full journeys A–D (failed agent, interrupt, commit+undo+recover) remain a human checklist. |
| AI-agent onboarding | **OBSERVATION.** Once before tag: one fresh coding agent, public README only. Not an LLM benchmark. See below. |

## Launch blockers

Open:

1. Full human journeys A–D beyond the darwin/arm64 T1–T4 smoke (failed agent; interrupt; commit+undo+recover).
2. Human ship/no-ship for announcement (none in v0.1; tag-and-stop).

Resolved:

- Overlay fail-closed on type-changed path with uncaptured children (now restores).
- README/help/contract aligned with binary undo output and wrapper-only stance.
- Reproducible `v0.1.0` tag (`978057fe`); GitHub Release four binaries + `checksums.txt`.
- T1–T4 against the GitHub Release on darwin/arm64.

## Non-blocking observations

- Detached HEAD / deleted-branch restore paths are implemented and fail closed; add tests if those workflows show up in real usage.
- Git index fidelity is a standing limitation, not a bug.
- `undo` vs `recover` success formatting differs.
- Lock does not stop a second tool that ignores flock.
- Process group is best-effort for grandchildren.
- No in-product telemetry (intentional).
- Demo counts in the README are illustrative of shape; live receipts win.
- Windows remains unsupported.
- **AI-agent onboarding comprehension.** Once before tag, give one fresh coding agent only the public README and ask it to explain/use the product. Record whether it infers: (1) user-level CLI, (2) no `init`, (3) no repo-local checkpoint store, (4) `doctor` is inspection, (5) `run` starts protection, (6) checkpoint storage is outside the repository. A miss is **OBSERVATION**, not a launch blocker: fix the copy. Do not add telemetry, agent heuristics, or an LLM CI harness.

## Commands run for this audit

```
go test ./...
go test -race ./...
go vet ./...
```

plus restore probes for type replacement, uncaptured children, and symlink non-follow.

Do not tag `v0.1.0` until the human launch gate in [v0.1-contract.md](v0.1-contract.md) is checked.

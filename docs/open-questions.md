# Open questions

Resolved for v0.1. Do not re-open in code; change the ADR first.

| ID | Decision | Source |
|---|---|---|
| Q1 Git undo | Ctrl+Z the repo: recovery checkpoint, plan, HEAD move if needed, **then overlay**. Never silent history rewrite. | [ADR 0002](adr/0002-git-restore-policy.md) |
| Q2 Ignore / capture | Tracked uncapped; untracked/ignored 1 MiB; skip bulky trees; gitignored ≠ irrelevant. | [ADR 0003](adr/0003-ignore-policy.md) |
| Q3 Git required | No. Non-git → filesystem only. Receipt: `Git: not captured`. | ADR 0002 |
| Q4 Created files | Exclusive session. Deletes listed in plan. No ownership inference. | [architecture.md](architecture.md) |
| Q5 Symlinks | Store as links. Do not follow out. No external copy. | [security-model.md](security-model.md) |
| Q6 Concurrency | One exclusive lock for `run`, `undo`, and `recover`. No silent steal. | architecture.md |
| Q7 Windows | Unsupported in v0.1. CI: ubuntu + macos. | [limitations.md](limitations.md) |
| Q8 ADR numbers | Keep `0001-go-for-core`. Git is `0002`. Ignore is `0003`. Session state is `0004`. | this lock |
| Q9 Process vs wrapper exit | Preserve child exit N. Interrupt 130. Usage 1. Internal 3. Attribution is the session record, not a reserved exit range. | [ADR 0004](adr/0004-session-state.md) |
| Q10 Session id | `XXXXXXXX-XXXXXXXX` uppercase hex. CLI takes session ids. `cp_…` rejected at CLI. | ADR 0004 |
| Q11 Argv / env | Persist argv as executed. Persist no environment. No fake redaction. | ADR 0004 |
| Q12 stdout/stderr | Inherit; do not store in the session record. | ADR 0004 |
| Q13 Process group | Dedicated pgid. SIGINT → TERM → KILL. Grandchildren best-effort. | ADR 0004 |
| Q14 keep | Not a command. Receipt language only. | ADR 0004 |

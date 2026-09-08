# Security

Agent Undo is a data-safety tool, not a sandbox. Report issues that could cause **incorrect restore**, **path escape**, **silent history rewrite**, or **checkpoint integrity** failures.

Do not report “the agent can delete `~/.agent-undo`” as a sandbox bypass. Same-uid agents can always do that; it is documented in [docs/security-model.md](docs/security-model.md).

## Supported in v0.1

macOS and Linux. Windows is unsupported.

## Please include

- Agent Undo version (`agent-undo version`)
- OS
- Whether the workspace is a git repo
- Exact command
- Whether restore claimed success (it must not, until verify exists)

## Rules we already treat as bugs

- Write outside the repository boundary
- Following a symlink out of the repo during traverse
- Claiming restore success without verification
- `undo` without a recovery checkpoint
- Force-push, branch deletion, or prune as part of undo

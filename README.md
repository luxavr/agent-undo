# Agent Undo

## Ctrl+Z for AI agents.

Checkpoint a local workspace, wrap an AI coding agent, see what changed, and restore the pre-session state.

```text
Clean repo
```

```bash
cd "$(./examples/setup-demo)"
agent-undo run ./demo-agent
```

The agent mutates the repository. The receipt reports what actually happened (counts come from the session diff, not a screenshot contract):

```text
Changes:
  +3 created
  ~3 modified
  -1 deleted

Undo:
  AVAILABLE
```

```bash
agent-undo undo --yes <session-id>
```

```text
SUCCESS
recovery checkpoint: cp_…
```

```text
Ctrl+Z for AI agents.
```

v0.1 is wrapper-based. Cursor/editor-native attachment is not supported.

Public contract: [docs/v0.1-contract.md](docs/v0.1-contract.md). Detailed annex: [docs/limitations.md](docs/limitations.md).

### Install

From a checkout (Go 1.23+, macOS or Linux):

```bash
go build -o bin/agent-undo ./cmd/agent-undo
export PATH="$PWD/bin:$PATH"
agent-undo doctor
```

There is no Homebrew formula and no install script in v0.1.

### Run

```bash
agent-undo run claude
agent-undo run -- your-agent --flags
```

`run` takes a lock, writes a verified checkpoint, starts the command in its own process group, then prints a receipt. `run` and `session show` print the same receipt.

### Undo

```bash
agent-undo session show <session-id>
agent-undo diff <session-id>
agent-undo undo [--yes] <session-id>
agent-undo verify <session-id>
```

Undo always creates a recovery checkpoint before APPLY. To put the post-agent state back:

```bash
agent-undo recover [--yes]
agent-undo recover [--yes] cp_<id>
```

### How it works

1. Checkpoint supported local files and git metadata (HEAD + branch, when present).
2. Run the agent as a child process.
3. Record a final manifest and a file/state diff.
4. Print an honest receipt.
5. On undo: lock → load → validate → plan → **recovery checkpoint** → apply → verify.

The checkpoint is the source of truth. Git is one dimension of state, not the whole state.

Canonical demo (disposable git repo, never this source tree):

```bash
go build -o bin/agent-undo ./cmd/agent-undo
export PATH="$PWD/bin:$PATH"
demo=$(./examples/setup-demo)
./examples/setup-demo --check
cd "$demo"
agent-undo run ./demo-agent
agent-undo undo --yes <session-id>
agent-undo verify <session-id>
```

`./demo-agent` refuses to run unless `.agent-undo-demo` is present. Default mutation is modify / create / delete. `--commit` and `--branch` are opt-in.

### What it restores

Supported local filesystem bytes inside the repository boundary, plus captured git HEAD/branch when the workspace was a git repo. Gitignored small files such as `.env.local` are included. See [docs/adr/0003-ignore-policy.md](docs/adr/0003-ignore-policy.md).

### What it does not restore

External side effects, cloud APIs, remote git, databases that were not checkpointed files, production infrastructure, git index/staging, `node_modules`-class trees, or anything outside the repository boundary. Agent Undo is not a sandbox.

Full list: [docs/limitations.md](docs/limitations.md).

### Supported platforms

macOS and Linux. Windows is unsupported in v0.1.

### Security model

Runs as the user. Paths are canonicalized inside one repository boundary. Symlinks are stored as links and are not followed out of the boundary. Destructive restore never starts without a verified recovery checkpoint.

[docs/security-model.md](docs/security-model.md)

### Architecture

[docs/architecture.md](docs/architecture.md) · [docs/adr/0002-git-restore-policy.md](docs/adr/0002-git-restore-policy.md) · [docs/adr/0004-session-state.md](docs/adr/0004-session-state.md)

Commands: `run`, `session list`, `session show`, `undo`, `verify`, `diff`, `recover`, `doctor`.

### Development

```bash
go test ./...
go test -race ./...
go vet ./...
```

CI: Ubuntu and macOS. Feature freeze: v0.1. Do not add Batch 8 subsystems. Report issues with the GitHub templates. Data-safety and restore-correctness outrank stars.

Launch gate and audit: [docs/v0.1-contract.md](docs/v0.1-contract.md), [docs/prelaunch-audit.md](docs/prelaunch-audit.md).

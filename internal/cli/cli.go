package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/luxavr/agent-undo/internal/version"
)

const (
	exitOK    = 0
	exitUsage = 1
)

const helpText = `Agent Undo — Ctrl+Z for a wrapped terminal AI coding session.

USAGE
  agent-undo <command> [arguments]

COMMANDS
  run <agent command>     Wrap an agent command under a checkpoint
  session list            List sessions
  session show <id>       Show session receipt and metadata
  undo [--yes] <session-id>
  verify <session-id>     Verify workspace against the session checkpoint
  diff <session-id>       Show file/state delta for a session
  recover [--yes] [cp_<id>]  Restore a recovery checkpoint
  doctor                  Inspect this directory and report whether this environment
                          can keep Agent Undo's promises. Does not initialize the repository.
  version                 Print version

v0.1 is wrapper-based. Cursor/editor-native attachment is not supported.

Session ids look like XXXXXXXX-XXXXXXXX. Checkpoint ids (cp_…) are not accepted
by undo/verify/diff/show. recover accepts only kind=recovery checkpoint ids.
--yes skips confirmation only; recover --yes requires an explicit cp_ id.
run refuses to checkpoint the user home directory.

Exit status of a completed child is preserved. Interrupted sessions exit 130.
Agent Undo internal failures exit 3. Exit status alone does not attribute the
failure; use session show.

MVP restores supported local filesystem state and git metadata.
It does not reverse cloud APIs, email, or other external side effects.
`

// Run dispatches the canonical CLI. args are os.Args[1:].
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, helpText)
		return exitOK
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, helpText)
		return exitOK
	case "-v", "--version", "version":
		fmt.Fprintln(stdout, version.Version)
		return exitOK
	case "run":
		return cmdRun(ctx, args[1:], stdout, stderr)
	case "session":
		return cmdSession(ctx, args[1:], stdout, stderr)
	case "undo":
		return cmdUndo(ctx, args[1:], stdout, stderr)
	case "verify":
		return cmdVerify(ctx, args[1:], stdout, stderr)
	case "diff":
		return cmdDiff(ctx, args[1:], stdout, stderr)
	case "recover":
		return cmdRecover(ctx, args[1:], stdout, stderr)
	case "doctor":
		return cmdDoctor(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return exitUsage
	}
}

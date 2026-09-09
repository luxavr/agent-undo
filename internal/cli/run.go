package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/luxavr/agent-undo/internal/checkpoint"
	"github.com/luxavr/agent-undo/internal/diff"
	"github.com/luxavr/agent-undo/internal/security"
	"github.com/luxavr/agent-undo/internal/session"
	"github.com/luxavr/agent-undo/internal/storage"
	"github.com/luxavr/agent-undo/internal/verify"
)

const exitInternal = 3

func cmdRun(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: agent-undo run [--] <agent command>\n")
		return exitUsage
	}
	store, b, err := openCWD()
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitInternal
	}
	if home, err := os.UserHomeDir(); err == nil && security.SameDirectory(b.Root(), home) {
		fmt.Fprintf(stderr, "run: refusing to checkpoint the home directory. cd to a project first.\n")
		return exitUsage
	}
	res := session.Wrap(ctx, session.WrapOptions{
		Boundary: b,
		Store:    store,
		Argv:     args,
		Stdout:   stdout,
		Stderr:   stderr,
	})
	if res.Record.AgentUndo.Error != "" && res.WrapperExit == exitInternal {
		if res.Record.Outcome == session.OutcomeAgentUndoError && strings.Contains(res.Record.AgentUndo.Error, session.NestedMessage()) {
			fmt.Fprintln(stderr, session.NestedMessage())
		} else {
			fmt.Fprintf(stderr, "run: %s\n", res.Record.AgentUndo.Error)
		}
		if res.Record.ID != "" {
			fmt.Fprintf(stderr, "session: %s\n", res.Record.ID)
		}
	}
	if res.Record.ID != "" {
		fmt.Fprint(stdout, session.FormatShow(store, res.Record))
	}
	return res.WrapperExit
}

func cmdSession(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: agent-undo session list|show <id>\n")
		return exitUsage
	}
	store, _, err := openCWD()
	if err != nil {
		fmt.Fprintf(stderr, "session: %v\n", err)
		return exitInternal
	}
	switch args[0] {
	case "list":
		recs, err := session.ListRecords(store)
		if err != nil {
			fmt.Fprintf(stderr, "session list: %v\n", err)
			return exitInternal
		}
		fmt.Fprint(stdout, session.FormatList(recs))
		return exitOK
	case "show":
		if len(args) != 2 {
			fmt.Fprintf(stderr, "usage: agent-undo session show <session-id>\n")
			return exitUsage
		}
		rec, err := loadSession(store, args[1], session.OpShow)
		if err != nil {
			fmt.Fprintf(stderr, "session show: %v\n", err)
			return exitUsage
		}
		fmt.Fprint(stdout, session.FormatShow(store, rec))
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown session command %q\n", args[0])
		return exitUsage
	}
}

func cmdDiff(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: agent-undo diff <session-id>\n")
		return exitUsage
	}
	store, _, err := openCWD()
	if err != nil {
		fmt.Fprintf(stderr, "diff: %v\n", err)
		return exitInternal
	}
	rec, err := loadSession(store, args[0], session.OpDiff)
	if err != nil {
		fmt.Fprintf(stderr, "diff: %v\n", err)
		return exitUsage
	}
	before, err := checkpoint.Load(store, rec.CheckpointID)
	if err != nil {
		fmt.Fprintf(stderr, "diff: %v\n", err)
		return exitInternal
	}
	after, err := checkpoint.Load(store, rec.FinalManifestID)
	if err != nil {
		fmt.Fprintf(stderr, "diff: %v\n", err)
		return exitInternal
	}
	dr, err := diff.Compare(before, after)
	if err != nil {
		fmt.Fprintf(stderr, "diff: %v\n", err)
		return exitInternal
	}
	fmt.Fprintf(stdout, "added: %d\nmodified: %d\ndeleted: %d\ntype-changed: %d\nsymlink-changed: %d\n",
		dr.Count(diff.Added), dr.Count(diff.Modified), dr.Count(diff.Deleted),
		dr.Count(diff.TypeChanged), dr.Count(diff.SymlinkChanged))
	for _, e := range dr.Entries {
		if e.Class != diff.Unchanged {
			fmt.Fprintf(stdout, "%s %s\n", e.Class, e.Path)
		}
	}
	return exitOK
}

func loadSession(store *storage.Store, id string, op session.Op) (session.Record, error) {
	if session.LooksLikeCheckpointID(id) {
		return session.Record{}, fmt.Errorf("requires a session id (XXXXXXXX-XXXXXXXX), not a checkpoint id. This is a recovery checkpoint. Use agent-undo recover to restore it")
	}
	rec, err := session.LoadRecord(store, id)
	if err != nil {
		return session.Record{}, err
	}
	if !rec.Allows(op) {
		if op == session.OpUndo && rec.CheckpointID != "" && !rec.ProcessStarted() {
			return session.Record{}, fmt.Errorf("session %s did not start a wrapped process; undo is not available", rec.ID)
		}
		return session.Record{}, fmt.Errorf("session %s state %s does not allow %s", rec.ID, rec.State, op)
	}
	return rec, nil
}

func lockedErr(err error) bool {
	return err != nil && (errors.Is(err, storage.ErrLocked) || strings.Contains(err.Error(), "repository locked"))
}

func cmdVerify(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintf(stderr, "usage: agent-undo verify <session-id>\n")
		return exitUsage
	}
	store, b, err := openCWD()
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitInternal
	}
	rec, err := loadSession(store, args[0], session.OpVerify)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitUsage
	}
	target, err := checkpoint.Load(store, rec.CheckpointID)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitInternal
	}
	if err := checkpoint.VerifyComplete(store, target); err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitInternal
	}
	if target.RepoRoot != b.Root() {
		fmt.Fprintf(stderr, "verify: repo root mismatch\n")
		return exitInternal
	}
	vr, err := verify.Workspace(ctx, target, b)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitInternal
	}
	if vr.Status != verify.StatusSuccess {
		fmt.Fprintf(stderr, "FAILED\n")
		for _, m := range vr.Mismatches {
			fmt.Fprintf(stderr, "  %s\n", m)
		}
		return exitUsage
	}
	fmt.Fprintln(stdout, "SUCCESS")
	return exitOK
}

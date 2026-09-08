package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/luxavr/agent-undo/internal/checkpoint"
	"github.com/luxavr/agent-undo/internal/restore"
	"github.com/luxavr/agent-undo/internal/security"
	"github.com/luxavr/agent-undo/internal/session"
	"github.com/luxavr/agent-undo/internal/storage"
)

func cmdUndo(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	id, yes, err := parseYesID(args)
	if err != nil {
		fmt.Fprintf(stderr, "usage: agent-undo undo [--yes] <session-id>\n")
		return exitUsage
	}
	store, b, err := openCWD()
	if err != nil {
		fmt.Fprintf(stderr, "undo: %v\n", err)
		return exitInternal
	}
	rec, err := loadSession(store, id, session.OpUndo)
	if err != nil {
		fmt.Fprintf(stderr, "undo: %v\n", err)
		return exitUsage
	}
	opts := restore.Options{
		Boundary: b, Store: store, TargetID: rec.CheckpointID, Yes: yes,
		RecoverySource: checkpoint.Source{
			Type:         checkpoint.SourceUndo,
			SessionID:    rec.ID,
			CheckpointID: rec.CheckpointID,
		},
	}
	if !yes {
		if isTTY(os.Stdin) {
			opts.Confirm = func(plan string) (bool, error) {
				fmt.Fprint(stdout, plan)
				fmt.Fprint(stdout, "\nProceed? [y/N] ")
				sc := bufio.NewScanner(os.Stdin)
				if !sc.Scan() {
					return false, sc.Err()
				}
				line := strings.TrimSpace(strings.ToLower(sc.Text()))
				return line == "y" || line == "yes", nil
			}
		}
	}
	rep := restore.Run(ctx, opts)
	if !yes && opts.Confirm == nil && rep.PlanText != "" {
		fmt.Fprint(stdout, rep.PlanText)
		fmt.Fprint(stdout, "\nProceed? [y/N]\n")
	}
	if lockedErr(rep.Err) {
		fmt.Fprintln(stderr, session.NestedMessage())
		return exitInternal
	}
	switch rep.Outcome {
	case restore.OutcomeSuccess:
		fmt.Fprintln(stdout, "SUCCESS")
		if rep.RecoveryID != "" {
			fmt.Fprintf(stdout, "recovery checkpoint: %s\n", rep.RecoveryID)
		}
		return exitOK
	case restore.OutcomeAborted:
		fmt.Fprintln(stderr, "restore aborted (pass --yes to run non-interactively)")
		return exitUsage
	default:
		if rep.Err != nil {
			fmt.Fprintf(stderr, "restore %s: %v\n", rep.Outcome, rep.Err)
		} else {
			fmt.Fprintf(stderr, "restore %s\n", rep.Outcome)
		}
		for _, m := range rep.Verify.Mismatches {
			fmt.Fprintf(stderr, "  %s\n", m)
		}
		if rep.RecoveryID != "" {
			fmt.Fprintf(stderr, "recovery checkpoint: %s\n", rep.RecoveryID)
			fmt.Fprintf(stderr, "next: %s\n", restore.RecoverHint(rep.RecoveryID))
		}
		return exitUsage
	}
}

func openCWD() (*storage.Store, *security.Boundary, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	home, err := storage.DefaultHome()
	if err != nil {
		return nil, nil, err
	}
	store, err := storage.Open(home)
	if err != nil {
		return nil, nil, err
	}
	b, err := security.NewBoundary(wd)
	if err != nil {
		return nil, nil, err
	}
	return store, b, nil
}

func parseYesID(args []string) (id string, yes bool, err error) {
	for _, a := range args {
		switch {
		case a == "--yes":
			yes = true
		case strings.HasPrefix(a, "-"):
			return "", false, fmt.Errorf("unknown flag")
		case id != "":
			return "", false, fmt.Errorf("extra argument")
		default:
			id = a
		}
	}
	if id == "" {
		return "", false, fmt.Errorf("missing id")
	}
	return id, yes, nil
}

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

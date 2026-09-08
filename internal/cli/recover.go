package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/idlfirhan/agent-undo/internal/checkpoint"
	"github.com/idlfirhan/agent-undo/internal/restore"
	"github.com/idlfirhan/agent-undo/internal/session"
	"github.com/idlfirhan/agent-undo/internal/verify"
)

func cmdRecover(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	id, yes, err := parseRecoverArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "usage: agent-undo recover [--yes] [cp_<id>]\n")
		return exitUsage
	}
	if id != "" {
		if _, err := session.CanonicalSessionID(id); err == nil {
			fmt.Fprintf(stderr, "recover: requires a recovery checkpoint id (cp_…), not a session id\n")
			return exitUsage
		}
		if !session.LooksLikeCheckpointID(id) {
			fmt.Fprintf(stderr, "recover: requires a recovery checkpoint id (cp_…)\n")
			return exitUsage
		}
	}
	store, b, err := openCWD()
	if err != nil {
		fmt.Fprintf(stderr, "recover: %v\n", err)
		return exitInternal
	}
	var target *checkpoint.Manifest
	if id == "" {
		target, err = checkpoint.LatestRecovery(store, b.Root())
		if errors.Is(err, checkpoint.ErrNoRecovery) {
			fmt.Fprintln(stderr, checkpoint.ErrNoRecovery.Error())
			return exitUsage
		}
		if err != nil {
			fmt.Fprintf(stderr, "recover: %v\n", err)
			return exitInternal
		}
	} else {
		target, err = checkpoint.ResolveRecovery(store, id, b.Root())
		if errors.Is(err, checkpoint.ErrNotRecovery) {
			fmt.Fprintf(stderr, "recover: not a recovery checkpoint\n")
			return exitUsage
		}
		if errors.Is(err, checkpoint.ErrIncompleteRecovery) || errors.Is(err, checkpoint.ErrRepoMismatch) {
			fmt.Fprintln(stderr, "NOT READY TO RECOVER")
			fmt.Fprintf(stderr, "Reason:\n  %s\n", recoverReason(err))
			return exitInternal
		}
		if err != nil {
			fmt.Fprintf(stderr, "recover: %v\n", err)
			return exitInternal
		}
	}

	opts := restore.Options{
		Boundary: b, Store: store, TargetID: target.ID, Yes: yes,
		RecoverySource: checkpoint.Source{
			Type:         checkpoint.SourceRecover,
			CheckpointID: target.ID,
		},
		PlanHeader: recoverPlanHeader(target),
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
		fmt.Fprint(stdout, recoverSuccess(rep.RecoveryID))
		return exitOK
	case restore.OutcomeAborted:
		fmt.Fprintln(stderr, "restore aborted (pass --yes to run non-interactively)")
		return exitUsage
	default:
		if rep.RecoveryID != "" {
			fmt.Fprint(stderr, recoverIncomplete(rep))
			return exitUsage
		}
		fmt.Fprintln(stderr, "NOT READY TO RECOVER")
		if rep.Err != nil {
			fmt.Fprintf(stderr, "Reason:\n  %s\n", rep.Err)
		}
		return exitInternal
	}
}

func parseRecoverArgs(args []string) (id string, yes bool, err error) {
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
	return id, yes, nil
}

func recoverReason(err error) string {
	if errors.Is(err, checkpoint.ErrRepoMismatch) {
		return "repository root mismatch"
	}
	if errors.Is(err, checkpoint.ErrIncompleteRecovery) {
		return "checkpoint objects incomplete"
	}
	return err.Error()
}

func recoverPlanHeader(m *checkpoint.Manifest) string {
	created := m.CreatedAt.UTC().Format("2006-01-02 15:04")
	return fmt.Sprintf(`AGENT UNDO RECOVER

Target:
  %s

Target created:
  %s

Protection:
  new recovery checkpoint will be created first

This operation will replace the current local workspace with the
target recovery checkpoint.

`, m.ID, created)
}

func recoverSuccess(recoveryID string) string {
	return fmt.Sprintf(`RECOVER

✓ recovery checkpoint created
✓ target state applied
✓ verification passed

Recovery:
  %s

Status:
  SUCCESS
`, recoveryID)
}

func recoverIncomplete(rep *restore.Report) string {
	status := string(rep.Outcome)
	if rep.Verify.Status == verify.StatusIncomplete || rep.Outcome == restore.OutcomeIncomplete {
		status = "INCOMPLETE"
	} else if rep.Verify.Status != "" {
		status = string(rep.Verify.Status)
	}
	return fmt.Sprintf(`RECOVER

Status:
  %s

Recovery checkpoint:
  %s

Run:
  %s
`, status, rep.RecoveryID, restore.RecoverHint(rep.RecoveryID))
}

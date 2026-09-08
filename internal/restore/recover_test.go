package restore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idlfirhan/agent-undo/internal/checkpoint"
)

func TestRecoverRoundTripNonGit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "A")
	store, b := harness(t, root)
	cpA := mustCreate(t, store, b)

	mustWrite(t, filepath.Join(root, "a.txt"), "B")
	mustWrite(t, filepath.Join(root, "extra.txt"), "x")
	undo := Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: cpA.ID, Yes: true,
		RecoverySource: checkpoint.Source{Type: checkpoint.SourceUndo, SessionID: "A81F92C3-7B10D4E2", CheckpointID: cpA.ID},
	})
	if undo.Outcome != OutcomeSuccess {
		t.Fatalf("undo: %s %v", undo.Outcome, undo.Err)
	}
	assertFile(t, filepath.Join(root, "a.txt"), "A")
	recB, err := checkpoint.Load(store, undo.RecoveryID)
	if err != nil {
		t.Fatal(err)
	}
	if recB.Kind != checkpoint.KindRecovery {
		t.Fatal(recB.Kind)
	}
	if recB.Source.Type != checkpoint.SourceUndo || recB.Source.SessionID != "A81F92C3-7B10D4E2" || recB.Source.CheckpointID != cpA.ID {
		t.Fatalf("lineage %+v", recB.Source)
	}

	latest, err := checkpoint.LatestRecovery(store, b.Root())
	if err != nil || latest.ID != recB.ID {
		t.Fatalf("latest %v %+v", err, latest)
	}

	rec := Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: recB.ID, Yes: true,
		RecoverySource: checkpoint.Source{Type: checkpoint.SourceRecover, CheckpointID: recB.ID},
		PlanHeader:     "AGENT UNDO RECOVER\n",
	})
	if rec.Outcome != OutcomeSuccess {
		t.Fatalf("recover: %s %v", rec.Outcome, rec.Err)
	}
	assertFile(t, filepath.Join(root, "a.txt"), "B")
	if _, err := os.Stat(filepath.Join(root, "extra.txt")); err != nil {
		t.Fatal("B extra.txt should be back")
	}
	recA, err := checkpoint.Load(store, rec.RecoveryID)
	if err != nil {
		t.Fatal(err)
	}
	if recA.Source.Type != checkpoint.SourceRecover || recA.Source.CheckpointID != recB.ID {
		t.Fatalf("recover lineage %+v", recA.Source)
	}

	again := Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: recB.ID, Yes: true,
		RecoverySource: checkpoint.Source{Type: checkpoint.SourceRecover, CheckpointID: recB.ID},
	})
	if again.Outcome != OutcomeSuccess {
		t.Fatalf("recover original B: %s %v", again.Outcome, again.Err)
	}
	assertFile(t, filepath.Join(root, "a.txt"), "B")

	mustWrite(t, filepath.Join(root, "a.txt"), "C")
	toA := Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: recA.ID, Yes: true,
		RecoverySource: checkpoint.Source{Type: checkpoint.SourceRecover, CheckpointID: recA.ID},
	})
	if toA.Outcome != OutcomeSuccess {
		t.Fatalf("recover A: %s %v", toA.Outcome, toA.Err)
	}
	assertFile(t, filepath.Join(root, "a.txt"), "A")
	if _, err := os.Stat(filepath.Join(root, "extra.txt")); !os.IsNotExist(err) {
		t.Fatal("extra from B should be gone at A")
	}
}

func TestRecoverCreatesCheckpointBeforeApply(t *testing.T) {
	t.Parallel()
	// CURRENT STATE → CREATE+VERIFY RECOVERY CHECKPOINT → APPLY TARGET → VERIFY.
	// Never APPLY first.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "A")
	store, b := harness(t, root)
	cpA := mustCreate(t, store, b)
	mustWrite(t, filepath.Join(root, "a.txt"), "B")
	undo := mustUndo(t, store, b, cpA.ID)

	sawRecovery := false
	rep := Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: undo.RecoveryID, Yes: true,
		After: func(point string) {
			if point == PointAfterRecovery {
				sawRecovery = true
				body, _ := os.ReadFile(filepath.Join(root, "a.txt"))
				if string(body) != "A" {
					t.Fatalf("must not APPLY before recovery checkpoint, got %q", body)
				}
			}
		},
	})
	if !sawRecovery || rep.RecoveryID == "" {
		t.Fatal("recovery checkpoint required")
	}
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v", rep.Outcome, rep.Err)
	}
	assertFile(t, filepath.Join(root, "a.txt"), "B")
}

func TestRecoverAbortedNoMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "A")
	store, b := harness(t, root)
	cpA := mustCreate(t, store, b)
	mustWrite(t, filepath.Join(root, "a.txt"), "B")
	undo := mustUndo(t, store, b, cpA.ID)
	rep := Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: undo.RecoveryID, Yes: false,
	})
	if rep.Outcome != OutcomeAborted {
		t.Fatalf("%s", rep.Outcome)
	}
	if rep.RecoveryID != "" {
		t.Fatal("no recovery checkpoint on abort")
	}
	assertFile(t, filepath.Join(root, "a.txt"), "A")
}

func TestRecoverPlanHeaderUsesExistingPlan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "A")
	store, b := harness(t, root)
	cpA := mustCreate(t, store, b)
	mustWrite(t, filepath.Join(root, "a.txt"), "B")
	undo := mustUndo(t, store, b, cpA.ID)
	var plan string
	_ = Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: undo.RecoveryID, Yes: false,
		PlanHeader: "AGENT UNDO RECOVER\n",
		Confirm: func(p string) (bool, error) {
			plan = p
			return false, nil
		},
	})
	if !strings.HasPrefix(plan, "AGENT UNDO RECOVER") {
		t.Fatalf("%q", plan)
	}
	if !strings.Contains(plan, "RESTORE PLAN") || !strings.Contains(plan, "Filesystem:") {
		t.Fatalf("must reuse FormatPlan:\n%s", plan)
	}
}

func TestRecoverGitDirty(t *testing.T) {
	t.Parallel()
	root := gitRepo(t)
	mustWrite(t, filepath.Join(root, "tracked.txt"), "v1")
	gitRun(t, root, "add", "tracked.txt")
	gitRun(t, root, "commit", "-q", "-m", "init")
	mustWrite(t, filepath.Join(root, "tracked.txt"), "dirty")
	mustWrite(t, filepath.Join(root, "untracked.txt"), "u")
	store, b := harness(t, root)
	cpA := mustCreate(t, store, b)
	mustWrite(t, filepath.Join(root, "tracked.txt"), "agent")
	gitRun(t, root, "add", "tracked.txt")
	gitRun(t, root, "commit", "-q", "-m", "agent")
	undo := mustUndo(t, store, b, cpA.ID)
	assertFile(t, filepath.Join(root, "tracked.txt"), "dirty")
	rep := Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: undo.RecoveryID, Yes: true,
		RecoverySource: checkpoint.Source{Type: checkpoint.SourceRecover, CheckpointID: undo.RecoveryID},
	})
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v %v", rep.Outcome, rep.Err, rep.Verify.Mismatches)
	}
	assertFile(t, filepath.Join(root, "tracked.txt"), "agent")
}

func TestRecoverFailsClosedIfSelectedTargetInvalidAfterLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "A")
	store, b := harness(t, root)
	cpA := mustCreate(t, store, b)

	mustWrite(t, filepath.Join(root, "a.txt"), "B")
	undoB := mustUndo(t, store, b, cpA.ID)
	mustWrite(t, filepath.Join(root, "a.txt"), "C")
	undoC := mustUndo(t, store, b, cpA.ID)
	assertFile(t, filepath.Join(root, "a.txt"), "A")

	latest, err := checkpoint.LatestRecovery(store, b.Root())
	if err != nil || latest.ID != undoC.RecoveryID {
		t.Fatalf("latest should be C: %v %+v", err, latest)
	}

	selected := undoB.RecoveryID
	rep := Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: selected, Yes: true,
		After: func(point string) {
			if point == PointAfterLock {
				_ = os.Remove(filepath.Join(store.Home, "manifests", selected+".json"))
			}
		},
	})
	if rep.Outcome == OutcomeSuccess {
		t.Fatal("must fail closed after the selected target became invalid")
	}
	if rep.RecoveryID != "" {
		t.Fatal("must not create a recovery checkpoint or APPLY")
	}
	assertFile(t, filepath.Join(root, "a.txt"), "A")
}

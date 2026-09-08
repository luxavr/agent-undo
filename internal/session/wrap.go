package session

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/idlfirhan/agent-undo/internal/checkpoint"
	"github.com/idlfirhan/agent-undo/internal/diff"
	"github.com/idlfirhan/agent-undo/internal/process"
	"github.com/idlfirhan/agent-undo/internal/security"
	"github.com/idlfirhan/agent-undo/internal/storage"
)

const (
	nestedMsg     = "Agent Undo session already active for this repository."
	internalExit  = 3
	interruptExit = 130
)

// WrapOptions orchestrates a protected run. No restore logic lives here.
type WrapOptions struct {
	Boundary *security.Boundary
	Store    *storage.Store
	Argv     []string
	Stdout   io.Writer
	Stderr   io.Writer
	Grace    time.Duration
}

// WrapResult is the wrapper result. Use Record for attribution, not exit alone.
type WrapResult struct {
	Record      Record
	WrapperExit int
}

// Wrap runs LOCK → checkpoint → child → final manifest → persist → unlock.
func Wrap(ctx context.Context, opts WrapOptions) WrapResult {
	if opts.Boundary == nil || opts.Store == nil {
		return WrapResult{
			WrapperExit: internalExit,
			Record: Record{
				State:   StateFailed,
				Outcome: OutcomeAgentUndoError,
				AgentUndo: AgentUndoInfo{
					ErrorCode: internalExit,
					Error:     "boundary and store are required",
				},
			},
		}
	}
	now := time.Now().UTC()
	rec := Record{
		ID:        NewSessionID(),
		State:     StateCreated,
		RepoRoot:  opts.Boundary.Root(),
		Argv:      append([]string(nil), opts.Argv...),
		StartedAt: now,
	}
	_ = Save(opts.Store, rec)

	fail := func(outcome Outcome, err error, exit int) WrapResult {
		t := time.Now().UTC()
		rec.State = StateFailed
		rec.Outcome = outcome
		rec.CompletedAt = &t
		rec.AgentUndo.ErrorCode = internalExit
		if err != nil {
			rec.AgentUndo.Error = err.Error()
		}
		_ = Save(opts.Store, rec)
		return WrapResult{Record: rec, WrapperExit: exit}
	}

	lock, err := opts.Store.TryLock(opts.Boundary.Root())
	if err != nil {
		return fail(OutcomeAgentUndoError, fmt.Errorf("%s", nestedMsg), internalExit)
	}
	defer lock.Unlock()

	rec.State = StateCheckpointing
	_ = Save(opts.Store, rec)
	before, err := checkpoint.Create(ctx, checkpoint.Options{
		Boundary: opts.Boundary,
		Store:    opts.Store,
		Kind:     checkpoint.KindSession,
	})
	if err != nil {
		return fail(OutcomeCheckpointFailed, err, internalExit)
	}
	if err := checkpoint.VerifyComplete(opts.Store, before); err != nil {
		return fail(OutcomeCheckpointFailed, err, internalExit)
	}
	rec.CheckpointID = before.ID
	rec.GitBefore = before.Git

	rec.State = StateRunning
	_ = Save(opts.Store, rec)
	pr := process.Execute(ctx, process.Options{
		Argv:     opts.Argv,
		Dir:      opts.Boundary.Root(),
		Stdout:   opts.Stdout,
		Stderr:   opts.Stderr,
		ExtraEnv: []string{process.MarkerEnv + "=" + rec.ID},
		Grace:    opts.Grace,
	})
	if pr.StartErr != nil {
		return fail(OutcomeAgentUndoError, pr.StartErr, internalExit)
	}
	code := pr.ExitCode
	rec.Process.ExitCode = &code
	rec.Process.Signal = pr.Signal

	// Final snapshot must still run after interrupt; do not inherit cancel.
	after, err := checkpoint.Create(context.WithoutCancel(ctx), checkpoint.Options{
		Boundary: opts.Boundary,
		Store:    opts.Store,
		Kind:     checkpoint.KindFinal,
	})
	if err != nil || checkpoint.VerifyComplete(opts.Store, after) != nil {
		if err == nil {
			err = fmt.Errorf("final manifest incomplete")
		}
		t := time.Now().UTC()
		rec.State = StateFailed
		rec.Outcome = OutcomeFinalSnapshotFailed
		rec.CompletedAt = &t
		rec.AgentUndo.ErrorCode = internalExit
		rec.AgentUndo.Error = err.Error()
		_ = Save(opts.Store, rec)
		return WrapResult{Record: rec, WrapperExit: internalExit}
	}
	rec.FinalManifestID = after.ID
	rec.GitAfter = after.Git
	if dr, err := diff.Compare(before, after); err == nil {
		rec.FilesCreated = dr.Count(diff.Added)
		rec.FilesModified = dr.Count(diff.Modified) + dr.Count(diff.TypeChanged) + dr.Count(diff.SymlinkChanged)
		rec.FilesDeleted = dr.Count(diff.Deleted)
	}
	t := time.Now().UTC()
	rec.CompletedAt = &t
	if pr.Interrupted {
		rec.State = StateInterrupted
		rec.Outcome = OutcomeInterrupted
		_ = Save(opts.Store, rec)
		return WrapResult{Record: rec, WrapperExit: interruptExit}
	}
	rec.State = StateCompleted
	rec.Outcome = OutcomeChildExit
	_ = Save(opts.Store, rec)
	return WrapResult{Record: rec, WrapperExit: code}
}

// NestedMessage is the stable lock-contention text.
func NestedMessage() string { return nestedMsg }

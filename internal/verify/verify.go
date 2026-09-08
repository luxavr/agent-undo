package verify

import (
	"context"
	"fmt"

	"github.com/idlfirhan/agent-undo/adapters/git"
	"github.com/idlfirhan/agent-undo/internal/checkpoint"
	"github.com/idlfirhan/agent-undo/internal/diff"
	"github.com/idlfirhan/agent-undo/internal/security"
)

// Status is owned by verify, not by restore APPLY.
type Status string

const (
	StatusSuccess    Status = "SUCCESS"
	StatusFailed     Status = "FAILED"
	StatusIncomplete Status = "INCOMPLETE"
)

// Result is the truth about whether live state matches the target checkpoint.
type Result struct {
	Status     Status
	Files      diff.Result
	GitOK      bool
	GitReason  string
	Mismatches []string
}

// Workspace scans the live tree and compares it to target. Success means the
// documented v0.1 contract: checkpointed filesystem + captured HEAD (if any).
func Workspace(ctx context.Context, target *checkpoint.Manifest, b *security.Boundary) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{Status: StatusIncomplete}, err
	}
	if target == nil || b == nil {
		return Result{Status: StatusFailed}, fmt.Errorf("verify: target and boundary are required")
	}
	live, err := checkpoint.Scan(ctx, b)
	if err != nil {
		return Result{Status: StatusFailed}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{Status: StatusIncomplete}, err
	}
	files, err := diff.Compare(target, live)
	if err != nil {
		return Result{Status: StatusFailed}, err
	}
	r := Result{Files: files, GitOK: true}
	if !files.Equal() {
		for _, e := range files.Entries {
			if e.Class != diff.Unchanged {
				r.Mismatches = append(r.Mismatches, fmt.Sprintf("%s %s", e.Class, e.Path))
			}
		}
	}
	if target.Git.Captured {
		st, err := git.Inspect(ctx, b.Root())
		if err != nil {
			return Result{Status: StatusFailed}, err
		}
		if !st.Captured || st.Head != target.Git.Head {
			r.GitOK = false
			r.GitReason = fmt.Sprintf("HEAD %s != captured %s", st.Head, target.Git.Head)
			r.Mismatches = append(r.Mismatches, r.GitReason)
		} else if !target.Git.Detached && target.Git.Branch != "" && st.Branch != target.Git.Branch {
			r.GitOK = false
			r.GitReason = fmt.Sprintf("branch %s != captured %s", st.Branch, target.Git.Branch)
			r.Mismatches = append(r.Mismatches, r.GitReason)
		}
	}
	if len(r.Mismatches) == 0 {
		r.Status = StatusSuccess
		return r, nil
	}
	r.Status = StatusFailed
	return r, nil
}

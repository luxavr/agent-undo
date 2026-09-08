package restore

import (
	"context"
	"fmt"
	"strings"

	"github.com/luxavr/agent-undo/adapters/git"
	"github.com/luxavr/agent-undo/internal/checkpoint"
	"github.com/luxavr/agent-undo/internal/diff"
	"github.com/luxavr/agent-undo/internal/security"
	"github.com/luxavr/agent-undo/internal/storage"
	"github.com/luxavr/agent-undo/internal/verify"
)

// Stage is one step of the restore pipeline. Order is invariant.
type Stage string

const (
	StageLock               Stage = "LOCK"
	StageLoad               Stage = "LOAD"
	StageValidate           Stage = "VALIDATE"
	StagePlan               Stage = "PLAN"
	StageRecoveryCheckpoint Stage = "RECOVERY CHECKPOINT"
	StageApply              Stage = "APPLY"
	StageVerify             Stage = "VERIFY"
	StageReport             Stage = "REPORT"
)

// Pipeline is the only legal order for a destructive restore.
var Pipeline = []Stage{
	StageLock,
	StageLoad,
	StageValidate,
	StagePlan,
	StageRecoveryCheckpoint,
	StageApply,
	StageVerify,
	StageReport,
}

const (
	PointBeforeLock      = "before lock"
	PointAfterLock       = "after lock"
	PointAfterLoad       = "after load"
	PointAfterValidation = "after validation"
	PointDuringPlan      = "during plan"
	PointBeforeRecovery  = "before recovery checkpoint"
	PointAfterRecovery   = "after recovery checkpoint"
	PointBeforeHEADMove  = "before HEAD move"
	PointAfterHEADMove   = "after HEAD move"
	PointBeforeOverlay   = "before overlay"
	PointDuringOverlay   = "during overlay"
	PointBeforeVerify    = "before verify"
	PointDuringVerify    = "during verify"
)

// Options for Run.
type Options struct {
	Boundary       *security.Boundary
	Store          *storage.Store
	TargetID       string
	Yes            bool
	Confirm        func(planText string) (bool, error)
	After          func(point string)
	RecoverySource checkpoint.Source
	PlanHeader     string
}

// Report is the restore outcome. Success is only StatusSuccess after verify.
type Report struct {
	Outcome    Outcome
	Plan       Plan
	PlanText   string
	RecoveryID string
	Verify     verify.Result
	Err        error
}

// Outcome is restore-level, distinct from verify.Status when aborted early.
type Outcome string

const (
	OutcomeSuccess    Outcome = "SUCCESS"
	OutcomeFailed     Outcome = "FAILED"
	OutcomeIncomplete Outcome = "INCOMPLETE"
	OutcomeAborted    Outcome = "ABORTED"
)

// Plan is shown before APPLY.
type Plan struct {
	RepoRoot     string
	Git          GitPlan
	Files        diff.Result
	Exclusions   []checkpoint.Exclusion
	DeletePaths  []string
	RecoveryHint string
}

// GitPlan is the mandatory git section (ADR 0002).
type GitPlan struct {
	Captured           GitRef
	Session            GitRef
	Proposed           GitRef
	GitCaptured        bool
	RewrittenHistory   bool
	SessionCommits     []string
	UnreachableMessage string
}

// GitRef is one HEAD/branch snapshot.
type GitRef struct {
	Head     string
	Branch   string
	Detached bool
}

type hookFn func(context.Context, string) error

func (o Options) hit(ctx context.Context, point string) error {
	if o.After != nil {
		o.After(point)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled at %s: %w", point, err)
	}
	return nil
}

// Run executes LOCK→LOAD→VALIDATE→PLAN→RECOVERY CHECKPOINT→APPLY→VERIFY→REPORT.
func Run(ctx context.Context, opts Options) *Report {
	rep := &Report{Outcome: OutcomeFailed}
	if err := opts.hit(ctx, PointBeforeLock); err != nil {
		return incomplete(rep, err)
	}
	if opts.Boundary == nil || opts.Store == nil || opts.TargetID == "" {
		rep.Err = fmt.Errorf("restore: boundary, store, and target id are required")
		return rep
	}
	lock, err := opts.Store.TryLock(opts.Boundary.Root())
	if err != nil {
		rep.Err = err
		return rep
	}
	defer lock.Unlock()
	if err := opts.hit(ctx, PointAfterLock); err != nil {
		return incomplete(rep, err)
	}

	target, err := checkpoint.Load(opts.Store, opts.TargetID)
	if err != nil {
		rep.Err = fmt.Errorf("load: %w", err)
		return rep
	}
	if err := checkpoint.VerifyComplete(opts.Store, target); err != nil {
		rep.Err = fmt.Errorf("validate objects: %w", err)
		return rep
	}
	if err := opts.hit(ctx, PointAfterLoad); err != nil {
		return incomplete(rep, err)
	}

	if err := validate(ctx, opts.Boundary, target); err != nil {
		rep.Err = err
		return rep
	}
	if err := opts.hit(ctx, PointAfterValidation); err != nil {
		return incomplete(rep, err)
	}

	live, err := checkpoint.Scan(ctx, opts.Boundary)
	if err != nil {
		rep.Err = fmt.Errorf("scan: %w", err)
		return rep
	}
	if err := opts.hit(ctx, PointDuringPlan); err != nil {
		return incomplete(rep, err)
	}
	plan, err := buildPlan(ctx, opts.Boundary.Root(), target, live)
	if err != nil {
		rep.Err = err
		return rep
	}
	rep.Plan = plan
	rep.PlanText = opts.PlanHeader + FormatPlan(plan)

	ok, err := confirm(opts, rep.PlanText)
	if err != nil {
		rep.Err = err
		return rep
	}
	if !ok {
		rep.Outcome = OutcomeAborted
		rep.Err = fmt.Errorf("restore aborted")
		return rep
	}

	if err := opts.hit(ctx, PointBeforeRecovery); err != nil {
		return incomplete(rep, err)
	}
	recovery, err := checkpoint.Create(ctx, checkpoint.Options{
		Boundary: opts.Boundary,
		Store:    opts.Store,
		Kind:     checkpoint.KindRecovery,
		Source:   opts.RecoverySource,
	})
	if err != nil {
		rep.Err = fmt.Errorf("recovery checkpoint: %w", err)
		return rep
	}
	if err := checkpoint.VerifyComplete(opts.Store, recovery); err != nil {
		rep.Err = fmt.Errorf("recovery checkpoint incomplete: %w", err)
		return rep
	}
	rep.RecoveryID = recovery.ID
	rep.Plan.RecoveryHint = recovery.ID
	rep.Plan.Git.UnreachableMessage = recovery.Git.Head
	if err := opts.hit(ctx, PointAfterRecovery); err != nil {
		return incomplete(rep, err)
	}

	if err := opts.hit(ctx, PointBeforeHEADMove); err != nil {
		return incomplete(rep, err)
	}
	if target.Git.Captured {
		if err := git.MoveHEAD(ctx, opts.Boundary.Root(), target.Git.Branch, target.Git.Detached, target.Git.Head); err != nil {
			rep.Outcome = OutcomeFailed
			rep.Err = fmt.Errorf("HEAD move: %w", err)
			return rep
		}
	}
	if err := opts.hit(ctx, PointAfterHEADMove); err != nil {
		return incomplete(rep, err)
	}

	if err := opts.hit(ctx, PointBeforeOverlay); err != nil {
		return incomplete(rep, err)
	}
	if err := overlay(ctx, opts, target, plan.DeletePaths); err != nil {
		rep.Outcome = OutcomeIncomplete
		rep.Err = err
		return rep
	}

	if err := opts.hit(ctx, PointBeforeVerify); err != nil {
		return incomplete(rep, err)
	}
	if err := opts.hit(ctx, PointDuringVerify); err != nil {
		return incomplete(rep, err)
	}
	vr, err := verify.Workspace(ctx, target, opts.Boundary)
	if err != nil {
		rep.Outcome = OutcomeFailed
		rep.Err = err
		rep.Verify = vr
		return rep
	}
	rep.Verify = vr
	if vr.Status != verify.StatusSuccess {
		rep.Outcome = OutcomeFailed
		rep.Err = fmt.Errorf("verify %s", vr.Status)
		return rep
	}
	rep.Outcome = OutcomeSuccess
	return rep
}

func incomplete(rep *Report, err error) *Report {
	rep.Outcome = OutcomeIncomplete
	rep.Err = err
	return rep
}

func confirm(opts Options, planText string) (bool, error) {
	if opts.Yes {
		return true, nil
	}
	if opts.Confirm != nil {
		return opts.Confirm(planText)
	}
	return false, nil
}

func validate(ctx context.Context, b *security.Boundary, m *checkpoint.Manifest) error {
	if m.RepoRoot != b.Root() {
		return fmt.Errorf("repo root mismatch: checkpoint %s, cwd %s", m.RepoRoot, b.Root())
	}
	if !m.Git.Captured {
		return nil
	}
	st, err := git.Inspect(ctx, b.Root())
	if err != nil {
		return err
	}
	if !st.Present {
		return fmt.Errorf("git restoration requested but .git is missing")
	}
	if m.Git.Unborn || m.Git.Head == "" {
		return fmt.Errorf("ambiguous git state: unborn or empty captured HEAD")
	}
	ok, err := git.ObjectExists(ctx, b.Root(), m.Git.Head)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("captured SHA %s is not in the object database", m.Git.Head)
	}
	if !m.Git.Detached && m.Git.Branch != "" {
		exists, err := git.BranchExists(ctx, b.Root(), m.Git.Branch)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("captured branch %q no longer exists", m.Git.Branch)
		}
	}
	return nil
}

func buildPlan(ctx context.Context, root string, target, live *checkpoint.Manifest) (Plan, error) {
	files, err := diff.Compare(target, live)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{
		RepoRoot:    root,
		Files:       files,
		Exclusions:  target.Exclusions,
		DeletePaths: files.Paths(diff.Added),
		Git: GitPlan{
			GitCaptured: target.Git.Captured,
			Captured:    gitRef(target.Git),
			Session:     gitRef(live.Git),
			Proposed:    gitRef(target.Git),
		},
	}
	if target.Git.Captured && live.Git.Captured && target.Git.Head != "" && live.Git.Head != "" && target.Git.Head != live.Git.Head {
		anc, err := git.IsAncestor(ctx, root, target.Git.Head, live.Git.Head)
		if err != nil {
			return Plan{}, err
		}
		if anc {
			list, err := git.RevList(ctx, root, target.Git.Head, live.Git.Head)
			if err != nil {
				return Plan{}, err
			}
			p.Git.SessionCommits = list
		} else {
			p.Git.RewrittenHistory = true
		}
	}
	return p, nil
}

func gitRef(g checkpoint.Git) GitRef {
	return GitRef{Head: g.Head, Branch: g.Branch, Detached: g.Detached}
}

// FormatPlan is the user-visible restore plan.
func FormatPlan(p Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "RESTORE PLAN\n\n")
	fmt.Fprintf(&b, "Repository:\n%s\n\n", p.RepoRoot)
	if p.Git.GitCaptured {
		fmt.Fprintf(&b, "Git:\n")
		fmt.Fprintf(&b, "CAPTURED   HEAD %s branch %s\n", short(p.Git.Captured.Head), branchLabel(p.Git.Captured))
		fmt.Fprintf(&b, "SESSION    HEAD %s branch %s\n", short(p.Git.Session.Head), branchLabel(p.Git.Session))
		fmt.Fprintf(&b, "PROPOSED   HEAD %s branch %s\n", short(p.Git.Proposed.Head), branchLabel(p.Git.Proposed))
		if p.Git.RewrittenHistory {
			fmt.Fprintf(&b, "history was rewritten; session commits cannot be listed linearly\n")
			fmt.Fprintf(&b, "session HEAD %s will leave the restored branch\n", short(p.Git.Session.Head))
		} else if n := len(p.Git.SessionCommits); n > 0 {
			fmt.Fprintf(&b, "commits leaving restored branch: %d\n", n)
		}
		fmt.Fprintln(&b)
	} else {
		fmt.Fprintf(&b, "Git: not captured\n\n")
	}
	fmt.Fprintf(&b, "Filesystem:\n")
	fmt.Fprintf(&b, "modified: %d\n", p.Files.Count(diff.Modified)+p.Files.Count(diff.TypeChanged)+p.Files.Count(diff.SymlinkChanged))
	fmt.Fprintf(&b, "created: %d\n", p.Files.Count(diff.Added))
	fmt.Fprintf(&b, "deleted: %d\n", p.Files.Count(diff.Deleted))
	fmt.Fprintf(&b, "symlinks: %d\n\n", p.Files.Count(diff.SymlinkChanged))
	fmt.Fprintf(&b, "Ignored files:\n")
	if len(p.Exclusions) == 0 {
		fmt.Fprintf(&b, "excluded: none\n\n")
	} else {
		names := make([]string, 0, len(p.Exclusions))
		for _, e := range p.Exclusions {
			names = append(names, e.Class)
		}
		fmt.Fprintf(&b, "excluded: %s\n\n", strings.Join(names, ", "))
	}
	if p.RecoveryHint != "" {
		fmt.Fprintf(&b, "Recovery:\npost-agent checkpoint: %s\n\n", p.RecoveryHint)
	}
	fmt.Fprintf(&b, "Risk:\n%d paths will be deleted\n", len(p.DeletePaths))
	if n := len(p.Git.SessionCommits); n > 0 {
		fmt.Fprintf(&b, "%d session commits will leave the restored branch\n", n)
	} else if p.Git.RewrittenHistory {
		fmt.Fprintf(&b, "rewritten session history will leave the restored branch\n")
	}
	return b.String()
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	if s == "" {
		return "(none)"
	}
	return s
}

func branchLabel(g GitRef) string {
	if g.Detached {
		return "(detached)"
	}
	if g.Branch == "" {
		return "(none)"
	}
	return g.Branch
}

// RecoverHint is the exact next command after a failed APPLY+VERIFY.
func RecoverHint(id string) string {
	return fmt.Sprintf("agent-undo recover %s", id)
}

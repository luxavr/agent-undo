package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/idlfirhan/agent-undo/internal/checkpoint"
	"github.com/idlfirhan/agent-undo/internal/diff"
	"github.com/idlfirhan/agent-undo/internal/storage"
)

const (
	StatusVerified    = "VERIFIED"
	StatusUnavailable = "UNAVAILABLE"
	StatusAvailable   = "AVAILABLE"
)

const scopeText = `Local checkpointed workspace state only.
  External side effects are not reversed.`

// Receipt is a projection of persisted session artifacts. It is not a
// source of truth and must not be built from the live workspace.
type Receipt struct {
	ID            string
	Command       string
	Duration      string
	Agent         string
	HasChanges    bool
	Created       int
	Modified      int
	Deleted       int
	ShowGit       bool
	GitCaptured   bool
	GitBranch     string
	GitBeforeHead string
	GitAfterHead  string
	Checkpoint    string
	Final         string
	Undo          string
	UndoReason    string
	NextDiff      bool
	NextUndo      bool
}

// LoadReceipt joins the session record with referenced manifests in the store.
// It does not scan the workspace, inspect live git, or read the clock.
func LoadReceipt(store *storage.Store, rec Record) Receipt {
	r := Receipt{
		ID:       rec.ID,
		Command:  commandLine(rec.Argv),
		Duration: durationOf(rec),
		Agent:    agentLine(rec),
	}
	cpOK, cpReason := manifestStatus(store, rec.CheckpointID)
	if cpOK {
		r.Checkpoint = StatusVerified
	} else {
		r.Checkpoint = StatusUnavailable
	}
	finOK, _ := manifestStatus(store, rec.FinalManifestID)
	switch {
	case rec.State == StateInterrupted:
		if finOK {
			r.Final = StatusVerified
		} else {
			r.Final = StatusUnavailable
		}
	case rec.Outcome == OutcomeFinalSnapshotFailed:
		r.Final = StatusUnavailable
	case rec.State == StateCompleted && rec.FinalManifestID != "" && !finOK:
		r.Final = StatusUnavailable
	}

	if cpOK && finOK {
		before, err1 := checkpoint.Load(store, rec.CheckpointID)
		after, err2 := checkpoint.Load(store, rec.FinalManifestID)
		if err1 == nil && err2 == nil {
			if dr, err := diff.Compare(before, after); err == nil {
				r.HasChanges = true
				r.Created = dr.Count(diff.Added)
				r.Modified = dr.Count(diff.Modified) + dr.Count(diff.TypeChanged) + dr.Count(diff.SymlinkChanged)
				r.Deleted = dr.Count(diff.Deleted)
			}
		}
	}

	if rec.GitBefore.Captured || rec.GitAfter.Captured {
		r.ShowGit = true
		r.GitCaptured = true
		r.GitBeforeHead = rec.GitBefore.Head
		r.GitAfterHead = rec.GitAfter.Head
		after := rec.GitAfter
		before := rec.GitBefore
		use := after
		if !after.Captured {
			use = before
		}
		if use.Detached || use.Branch == "" {
			r.GitBranch = "detached"
		} else {
			r.GitBranch = use.Branch
		}
	} else if rec.CheckpointID != "" {
		r.ShowGit = true
		r.GitCaptured = false
	}

	if rec.Allows(OpUndo) && cpOK {
		r.Undo = StatusAvailable
		r.NextUndo = true
	} else {
		r.Undo = StatusUnavailable
		r.UndoReason = undoReason(rec, cpOK, cpReason)
	}
	r.NextDiff = rec.Allows(OpDiff) && cpOK && finOK
	return r
}

func manifestStatus(store *storage.Store, id string) (ok bool, reason string) {
	if id == "" {
		return false, "no checkpoint"
	}
	if store == nil {
		return false, "checkpoint missing"
	}
	m, err := checkpoint.Load(store, id)
	if err != nil {
		return false, "checkpoint missing"
	}
	if err := checkpoint.VerifyComplete(store, m); err != nil {
		return false, "checkpoint verification failed"
	}
	return true, ""
}

func undoReason(rec Record, cpOK bool, cpReason string) string {
	switch rec.State {
	case StateCreated, StateCheckpointing:
		return "session not finished"
	case StateRunning:
		return "session still running"
	}
	if rec.Outcome == OutcomeCheckpointFailed {
		return "checkpoint verification failed"
	}
	if rec.CheckpointID == "" || !cpOK {
		if cpReason != "" {
			return cpReason
		}
		return "checkpoint verification failed"
	}
	return "undo not permitted"
}

func commandLine(argv []string) string {
	if len(argv) == 0 {
		return "-"
	}
	return strings.Join(argv, " ")
}

func durationOf(rec Record) string {
	if rec.CompletedAt == nil {
		return "-"
	}
	return formatDuration(rec.CompletedAt.Sub(rec.StartedAt))
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Truncate(time.Second)
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	s := int((d % time.Minute) / time.Second)
	if h > 0 {
		return fmt.Sprintf("%02dh %02dm %02ds", h, m, s)
	}
	return fmt.Sprintf("%02dm %02ds", m, s)
}

func agentLine(rec Record) string {
	switch rec.State {
	case StateInterrupted:
		return "INTERRUPTED"
	case StateRunning:
		return "running"
	case StateCreated, StateCheckpointing:
		return "not started"
	case StateCompleted:
		if rec.Process.ExitCode != nil {
			return fmt.Sprintf("exited %d", *rec.Process.ExitCode)
		}
		return "exited 0"
	default:
		if rec.Process.ExitCode != nil {
			return fmt.Sprintf("exited %d", *rec.Process.ExitCode)
		}
		return "not started"
	}
}

// Render formats a receipt. Pure: no IO, clock, or environment.
func Render(r Receipt) string {
	var b strings.Builder
	b.WriteString("AGENT UNDO\n\n")
	fmt.Fprintf(&b, "Session:\n  %s\n\n", dash(r.ID))
	fmt.Fprintf(&b, "Command:\n  %s\n\n", dash(r.Command))
	fmt.Fprintf(&b, "Duration:\n  %s\n\n", dash(r.Duration))
	fmt.Fprintf(&b, "Agent:\n  %s\n\n", dash(r.Agent))
	if r.HasChanges {
		fmt.Fprintf(&b, "Changes:\n  +%d created\n  ~%d modified\n  -%d deleted\n\n", r.Created, r.Modified, r.Deleted)
	}
	if r.ShowGit {
		b.WriteString("Git:\n")
		if !r.GitCaptured {
			b.WriteString("  not captured\n\n")
		} else {
			fmt.Fprintf(&b, "  %s\n", dash(r.GitBranch))
			switch {
			case r.GitBeforeHead != "" && r.GitAfterHead != "":
				fmt.Fprintf(&b, "  %s → %s\n\n", r.GitBeforeHead, r.GitAfterHead)
			case r.GitAfterHead != "":
				fmt.Fprintf(&b, "  %s\n\n", r.GitAfterHead)
			default:
				fmt.Fprintf(&b, "  %s\n\n", dash(r.GitBeforeHead))
			}
		}
	}
	fmt.Fprintf(&b, "Checkpoint:\n  %s\n\n", dash(r.Checkpoint))
	if r.Final != "" {
		fmt.Fprintf(&b, "Final snapshot:\n  %s\n\n", r.Final)
	}
	fmt.Fprintf(&b, "Undo:\n  %s\n\n", dash(r.Undo))
	if r.Undo == StatusUnavailable && r.UndoReason != "" {
		fmt.Fprintf(&b, "Reason:\n  %s\n\n", r.UndoReason)
	}
	fmt.Fprintf(&b, "Scope:\n  %s\n\n", scopeText)
	b.WriteString("Next:\n")
	if r.ID != "" {
		fmt.Fprintf(&b, "  agent-undo session show %s\n", r.ID)
		if r.NextDiff {
			fmt.Fprintf(&b, "  agent-undo diff %s\n", r.ID)
		}
		if r.NextUndo {
			fmt.Fprintf(&b, "  agent-undo undo %s\n", r.ID)
		}
	}
	return b.String()
}

// FormatShow renders the receipt for a persisted session.
func FormatShow(store *storage.Store, rec Record) string {
	return Render(LoadReceipt(store, rec))
}

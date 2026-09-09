package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/luxavr/agent-undo/internal/checkpoint"
	"github.com/luxavr/agent-undo/internal/storage"
)

// State is the session lifecycle state.
type State string

const (
	StateCreated       State = "CREATED"
	StateCheckpointing State = "CHECKPOINTING"
	StateRunning       State = "RUNNING"
	StateCompleted     State = "COMPLETED"
	StateInterrupted   State = "INTERRUPTED"
	StateFailed        State = "FAILED"
)

// Outcome is why the session ended (orthogonal to State).
type Outcome string

const (
	OutcomeChildExit           Outcome = "CHILD_EXIT"
	OutcomeInterrupted         Outcome = "INTERRUPTED"
	OutcomeAgentUndoError      Outcome = "AGENT_UNDO_ERROR"
	OutcomeFinalSnapshotFailed Outcome = "FINAL_SNAPSHOT_FAILED"
	OutcomeCheckpointFailed    Outcome = "CHECKPOINT_FAILED"
)

// Op is a session command.
type Op string

const (
	OpShow   Op = "show"
	OpDiff   Op = "diff"
	OpVerify Op = "verify"
	OpUndo   Op = "undo"
)

// Record is persisted session metadata. No environment. No stdout.
type Record struct {
	ID              string         `json:"id"`
	State           State          `json:"state"`
	Outcome         Outcome        `json:"outcome,omitempty"`
	RepoRoot        string         `json:"repoRoot"`
	Argv            []string       `json:"argv"`
	StartedAt       time.Time      `json:"startedAt"`
	CompletedAt     *time.Time     `json:"completedAt,omitempty"`
	CheckpointID    string         `json:"checkpointId,omitempty"`
	FinalManifestID string         `json:"finalManifestId,omitempty"`
	Process         ProcessInfo    `json:"process"`
	AgentUndo       AgentUndoInfo  `json:"agentUndo"`
	GitBefore       checkpoint.Git `json:"gitBefore"`
	GitAfter        checkpoint.Git `json:"gitAfter"`
	FilesCreated    int            `json:"filesCreated,omitempty"`
	FilesModified   int            `json:"filesModified,omitempty"`
	FilesDeleted    int            `json:"filesDeleted,omitempty"`
}

// ProcessInfo is the child result.
type ProcessInfo struct {
	// Started is true after process.Execute returns StartErr == nil.
	Started  bool   `json:"started,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
	Signal   string `json:"signal,omitempty"`
}

// AgentUndoInfo is the wrapper result. Distinct from ProcessInfo.
type AgentUndoInfo struct {
	ErrorCode int    `json:"errorCode,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ProcessStarted reports whether the wrapped process actually started
// (process.Execute returned StartErr == nil). v0.1.0 records did not persist
// Started; ExitCode set after a successful Start is the compatibility signal.
func (r Record) ProcessStarted() bool {
	if r.Process.Started {
		return true
	}
	return r.Process.ExitCode != nil
}

func terminalWithCheckpoint(r Record) bool {
	if r.CheckpointID == "" {
		return false
	}
	switch r.State {
	case StateCompleted, StateInterrupted, StateFailed:
		return true
	default:
		return false
	}
}

// Allows reports whether op is permitted (ADR 0004).
func (r Record) Allows(op Op) bool {
	switch op {
	case OpShow:
		return r.ID != ""
	case OpVerify:
		return terminalWithCheckpoint(r)
	case OpUndo:
		return terminalWithCheckpoint(r) && r.ProcessStarted()
	case OpDiff:
		return r.CheckpointID != "" && r.FinalManifestID != "" &&
			(r.State == StateCompleted || r.State == StateInterrupted || r.State == StateFailed)
	default:
		return false
	}
}

func sessionDir(store *storage.Store, id string) string {
	return filepath.Join(store.Home, "sessions", id)
}

func recordPath(store *storage.Store, id string) string {
	return filepath.Join(sessionDir(store, id), "record.json")
}

// Save writes record.json atomically.
func Save(store *storage.Store, rec Record) error {
	if _, err := CanonicalSessionID(rec.ID); err != nil {
		return err
	}
	dir := sessionDir(store, rec.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, recordPath(store, rec.ID))
}

// LoadRecord reads a session record.
func LoadRecord(store *storage.Store, id string) (Record, error) {
	id, err := CanonicalSessionID(id)
	if err != nil {
		return Record{}, err
	}
	raw, err := os.ReadFile(recordPath(store, id))
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// ListRecords returns sessions newest-first.
func ListRecords(store *storage.Store) ([]Record, error) {
	root := filepath.Join(store.Home, "sessions")
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Record
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		rec, err := LoadRecord(store, e.Name())
		if err != nil {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}

// FormatList is the minimal list view.
func FormatList(recs []Record) string {
	if len(recs) == 0 {
		return "no sessions\n"
	}
	s := "SESSION ID          STATE         COMMAND              START                END                  EXIT\n"
	for _, r := range recs {
		cmd := ""
		if len(r.Argv) > 0 {
			cmd = r.Argv[0]
		}
		end := "-"
		if r.CompletedAt != nil {
			end = r.CompletedAt.UTC().Format(time.RFC3339)
		}
		exit := "-"
		if r.Process.ExitCode != nil {
			exit = fmt.Sprintf("%d", *r.Process.ExitCode)
		}
		s += fmt.Sprintf("%-19s %-13s %-20s %-20s %-20s %s\n", r.ID, r.State, trunc(cmd, 20), r.StartedAt.UTC().Format(time.RFC3339), end, exit)
	}
	return s
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

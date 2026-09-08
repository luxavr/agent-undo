package session

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luxavr/agent-undo/internal/checkpoint"
	"github.com/luxavr/agent-undo/internal/storage"
)

func TestRenderCompleted(t *testing.T) {
	t.Parallel()
	id := "A81F92C3-7B10D4E2"
	got := Render(Receipt{
		ID:            id,
		Command:       "claude",
		Duration:      "04m 18s",
		Agent:         "exited 0",
		HasChanges:    true,
		Created:       4,
		Modified:      23,
		Deleted:       2,
		ShowGit:       true,
		GitCaptured:   true,
		GitBranch:     "main",
		GitBeforeHead: "abc123",
		GitAfterHead:  "def456",
		Checkpoint:    StatusVerified,
		Undo:          StatusAvailable,
		NextDiff:      true,
		NextUndo:      true,
	})
	want := `AGENT UNDO

Session:
  A81F92C3-7B10D4E2

Command:
  claude

Duration:
  04m 18s

Agent:
  exited 0

Changes:
  +4 created
  ~23 modified
  -2 deleted

Git:
  main
  abc123 → def456

Checkpoint:
  VERIFIED

Undo:
  AVAILABLE

Scope:
  Local checkpointed workspace state only.
  External side effects are not reversed.

Next:
  agent-undo session show A81F92C3-7B10D4E2
  agent-undo diff A81F92C3-7B10D4E2
  agent-undo undo A81F92C3-7B10D4E2
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "SUCCESS") || strings.Contains(got, "FAILED") {
		t.Fatal("receipt must not use SUCCESS/FAILED")
	}
	if strings.Contains(got, "[keep]") || strings.Contains(got, "[k]") {
		t.Fatal("no action loop")
	}
}

func TestRenderInterrupted(t *testing.T) {
	t.Parallel()
	id := "A81F92C3-7B10D4E2"
	got := Render(Receipt{
		ID:         id,
		Command:    "claude",
		Duration:   "00m 05s",
		Agent:      "INTERRUPTED",
		HasChanges: true,
		Created:    1,
		Checkpoint: StatusVerified,
		Final:      StatusVerified,
		Undo:       StatusAvailable,
		NextDiff:   true,
		NextUndo:   true,
	})
	if !strings.Contains(got, "Agent:\n  INTERRUPTED\n") {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(got, "Checkpoint:\n  VERIFIED\n") {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(got, "Final snapshot:\n  VERIFIED\n") {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(got, "Undo:\n  AVAILABLE\n") {
		t.Fatalf("%s", got)
	}
}

func TestRenderCheckpointFailed(t *testing.T) {
	t.Parallel()
	id := "A81F92C3-7B10D4E2"
	got := Render(Receipt{
		ID:         id,
		Command:    "claude",
		Duration:   "00m 01s",
		Agent:      "not started",
		Checkpoint: StatusUnavailable,
		Undo:       StatusUnavailable,
		UndoReason: "checkpoint verification failed",
	})
	want := `AGENT UNDO

Session:
  A81F92C3-7B10D4E2

Command:
  claude

Duration:
  00m 01s

Agent:
  not started

Checkpoint:
  UNAVAILABLE

Undo:
  UNAVAILABLE

Reason:
  checkpoint verification failed

Scope:
  Local checkpointed workspace state only.
  External side effects are not reversed.

Next:
  agent-undo session show A81F92C3-7B10D4E2
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "email") || strings.Contains(got, "Cannot undo:") {
		t.Fatal("must not enumerate unobserved side effects")
	}
}

func TestRenderDeterministic(t *testing.T) {
	t.Parallel()
	r := Receipt{ID: "A81F92C3-7B10D4E2", Command: "true", Duration: "00m 00s", Agent: "exited 0", Checkpoint: StatusVerified, Undo: StatusAvailable, NextUndo: true}
	if Render(r) != Render(r) {
		t.Fatal("render must be deterministic")
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	if formatDuration(4*time.Minute+18*time.Second) != "04m 18s" {
		t.Fatal(formatDuration(4*time.Minute + 18*time.Second))
	}
	if formatDuration(time.Hour+2*time.Minute+3*time.Second) != "01h 02m 03s" {
		t.Fatal(formatDuration(time.Hour + 2*time.Minute + 3*time.Second))
	}
}

func TestLoadReceiptUsesPersistedArtifactsOnly(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	end := start.Add(4*time.Minute + 18*time.Second)
	code := 0
	rec := Record{
		ID:           "A81F92C3-7B10D4E2",
		State:        StateCompleted,
		Outcome:      OutcomeChildExit,
		Argv:         []string{"claude"},
		StartedAt:    start,
		CompletedAt:  &end,
		Process:      ProcessInfo{ExitCode: &code},
		GitBefore:    checkpoint.Git{Captured: true, Branch: "main", Head: "abc123"},
		GitAfter:     checkpoint.Git{Captured: true, Branch: "main", Head: "def456"},
		FilesCreated: 4, FilesModified: 23, FilesDeleted: 2,
	}
	r := LoadReceipt(store, rec)
	if r.Agent != "exited 0" || r.Duration != "04m 18s" {
		t.Fatalf("%+v", r)
	}
	if r.Checkpoint != StatusUnavailable || r.Undo != StatusUnavailable {
		t.Fatalf("missing checkpoint must be unavailable: %+v", r)
	}
	if r.UndoReason != "no checkpoint" {
		t.Fatalf("reason %q", r.UndoReason)
	}
	if r.HasChanges {
		t.Fatal("must not invent a delta without verified manifests")
	}
}

func TestLoadReceiptRunningOmitsUndo(t *testing.T) {
	t.Parallel()
	r := LoadReceipt(nil, Record{
		ID:           "A81F92C3-7B10D4E2",
		State:        StateRunning,
		Argv:         []string{"sleep"},
		CheckpointID: "cp_dead",
		StartedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if r.Undo != StatusUnavailable || r.NextUndo {
		t.Fatalf("%+v", r)
	}
	if r.Agent != "running" {
		t.Fatalf("%q", r.Agent)
	}
}

func TestLoadReceiptJoinsVerifiedManifests(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.txt"), "a")
	store, b := harness(t, root)
	before, err := checkpoint.Create(context.Background(), checkpoint.Options{
		Boundary: b, Store: store, Kind: checkpoint.KindSession,
	})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "keep.txt"), "b")
	mustWrite(t, filepath.Join(root, "new.txt"), "n")
	after, err := checkpoint.Create(context.Background(), checkpoint.Options{
		Boundary: b, Store: store, Kind: checkpoint.KindFinal,
	})
	if err != nil {
		t.Fatal(err)
	}
	code := 0
	end := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	rec := Record{
		ID:              "A81F92C3-7B10D4E2",
		State:           StateCompleted,
		Outcome:         OutcomeChildExit,
		Argv:            []string{"claude"},
		StartedAt:       end.Add(-time.Minute),
		CompletedAt:     &end,
		CheckpointID:    before.ID,
		FinalManifestID: after.ID,
		Process:         ProcessInfo{ExitCode: &code},
		FilesCreated:    99,
		FilesModified:   99,
		FilesDeleted:    99,
	}
	r := LoadReceipt(store, rec)
	if r.Checkpoint != StatusVerified || r.Undo != StatusAvailable || !r.NextUndo || !r.NextDiff {
		t.Fatalf("%+v", r)
	}
	if !r.HasChanges || r.Created < 1 || r.Modified < 1 {
		t.Fatalf("compare counts %+v", r)
	}
	if r.Created == 99 || r.Modified == 99 {
		t.Fatal("must join manifests, not trust stale record counts when compare works")
	}
	got := Render(r)
	if strings.Contains(got, "SUCCESS") || strings.Contains(got, "FAILED") {
		t.Fatal(got)
	}
}

func TestLoadReceiptIncompleteCheckpoint(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteManifest("cp_incomplete", []byte(`{"schemaVersion":1,"id":"cp_incomplete","kind":"session","files":[{"path":"a","kind":"file","sha256":"aa","size":1}]}`+"\n")); err != nil {
		t.Fatal(err)
	}
	rec := Record{
		ID:           "A81F92C3-7B10D4E2",
		State:        StateFailed,
		Outcome:      OutcomeCheckpointFailed,
		Argv:         []string{"claude"},
		CheckpointID: "cp_incomplete",
		StartedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		FilesCreated: 7,
	}
	r := LoadReceipt(store, rec)
	if r.Checkpoint != StatusUnavailable || r.Undo != StatusUnavailable || r.HasChanges {
		t.Fatalf("%+v", r)
	}
	if r.UndoReason != "checkpoint verification failed" {
		t.Fatalf("%q", r.UndoReason)
	}
}

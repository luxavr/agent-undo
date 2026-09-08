package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idlfirhan/agent-undo/internal/security"
	"github.com/idlfirhan/agent-undo/internal/session"
	"github.com/idlfirhan/agent-undo/internal/storage"
)

func TestCLIRecoverNoCheckpoint(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGENT_UNDO_HOME", t.TempDir())
	chdir(t, root)
	_, stderr, code := run([]string{"recover", "--yes"})
	if code != exitUsage {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
	if strings.TrimSpace(stderr) != "no recovery checkpoint for this repository." {
		t.Fatalf("%q", stderr)
	}
}

func TestCLIRecoverRejectsSessionAndNonRecovery(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("AGENT_UNDO_HOME", home)
	chdir(t, root)

	_, stderr, code := run([]string{"recover", "--yes", session.NewSessionID()})
	if code != exitUsage || !strings.Contains(stderr, "session id") {
		t.Fatalf("%d %q", code, stderr)
	}

	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "agent.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf new > a.txt\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, _, code := run([]string{"run", script})
	if code != 0 {
		t.Fatalf("run %d %s", code, stdout)
	}
	id := sessionIDFromShow(t, stdout)
	store, err := storage.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := session.LoadRecord(store, id)
	if err != nil {
		t.Fatal(err)
	}
	_, stderr, code = run([]string{"recover", "--yes", rec.CheckpointID})
	if code != exitUsage || !strings.Contains(stderr, "not a recovery checkpoint") {
		t.Fatalf("%d %q", code, stderr)
	}

	stdout, stderr, code = run([]string{"undo", "--yes", id})
	if code != 0 {
		t.Fatalf("undo %d %q %q", code, stdout, stderr)
	}
	stdout, stderr, code = run([]string{"recover", "--yes"})
	if code != 0 {
		t.Fatalf("recover %d %q %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "RECOVER") || !strings.Contains(stdout, "SUCCESS") {
		t.Fatalf("%q", stdout)
	}
	body, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil || string(body) != "new" {
		t.Fatalf("got %q %v", body, err)
	}
}

func TestCLIRecoverIncomplete(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("AGENT_UNDO_HOME", home)
	chdir(t, root)
	store, err := storage.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"schemaVersion": 1,
		"id":            "cp_incomplete01",
		"kind":          "recovery",
		"repoRoot":      b.Root(),
		"files":         []map[string]any{{"path": "a", "kind": "file", "sha256": "aa", "size": 1}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := store.WriteManifest("cp_incomplete01", raw); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := run([]string{"recover", "--yes", "cp_incomplete01"})
	if code != exitInternal {
		t.Fatalf("%d %q", code, stderr)
	}
	if !strings.Contains(stderr, "NOT READY TO RECOVER") || !strings.Contains(stderr, "checkpoint objects incomplete") {
		t.Fatalf("%q", stderr)
	}
}

func TestCLIRecoverMissingExplicit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGENT_UNDO_HOME", t.TempDir())
	chdir(t, root)
	_, stderr, code := run([]string{"recover", "--yes", "cp_doesnotexist1"})
	if code != exitInternal {
		t.Fatalf("%d %q", code, stderr)
	}
	if !strings.Contains(stderr, "NOT READY TO RECOVER") {
		t.Fatalf("%q", stderr)
	}
}

func TestCLIRecoverNonTTYRequiresYes(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("AGENT_UNDO_HOME", home)
	chdir(t, root)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "agent.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf new > a.txt\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, _, code := run([]string{"run", script})
	if code != 0 {
		t.Fatal(stdout)
	}
	id := sessionIDFromShow(t, stdout)
	stdout, stderr, code := run([]string{"undo", "--yes", id})
	if code != 0 {
		t.Fatalf("undo %d %q %q", code, stdout, stderr)
	}
	stdout, stderr, code = run([]string{"recover"})
	if code != exitUsage {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "AGENT UNDO RECOVER") || !strings.Contains(stdout, "RESTORE PLAN") {
		t.Fatalf("plan:\n%s", stdout)
	}
	body, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(body) != "old" {
		t.Fatalf("must not apply without --yes: %q", body)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

package session

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luxavr/agent-undo/internal/checkpoint"
	"github.com/luxavr/agent-undo/internal/restore"
	"github.com/luxavr/agent-undo/internal/security"
	"github.com/luxavr/agent-undo/internal/storage"
)

func TestWrapHappyPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.txt"), "a")
	mustWrite(t, filepath.Join(root, "gone.txt"), "g")
	store, b := harness(t, root)
	script := filepath.Join(root, "agent.sh")
	mustWrite(t, script, "#!/bin/sh\nprintf mutated > keep.txt\nrm -f gone.txt\nprintf new > created.txt\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	res := Wrap(context.Background(), WrapOptions{
		Boundary: b, Store: store, Argv: []string{script},
		Stdout: io.Discard, Stderr: io.Discard,
	})
	if res.WrapperExit != 0 || res.Record.State != StateCompleted || res.Record.Outcome != OutcomeChildExit {
		t.Fatalf("%+v", res.Record)
	}
	if res.Record.CheckpointID == "" || res.Record.FinalManifestID == "" {
		t.Fatal("manifests")
	}
	before, err := checkpoint.Load(store, res.Record.CheckpointID)
	if err != nil || before.Kind != checkpoint.KindSession {
		t.Fatalf("before kind %v %v", before, err)
	}
	after, err := checkpoint.Load(store, res.Record.FinalManifestID)
	if err != nil || after.Kind != checkpoint.KindFinal {
		t.Fatalf("after kind %v %v", after, err)
	}
	if res.Record.FilesCreated < 1 || res.Record.FilesDeleted < 1 || res.Record.FilesModified < 1 {
		t.Fatalf("diff counts %+v", res.Record)
	}
	raw, err := os.ReadFile(filepath.Join(store.Home, "sessions", res.Record.ID, "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("HOME=")) || bytes.Contains(bytes.ToUpper(raw), []byte("API_KEY")) {
		t.Fatalf("must not persist env: %s", raw)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["env"]; ok {
		t.Fatal("record must not contain env")
	}
}

func TestWrapPersistsArgvLiterally(t *testing.T) {
	root := t.TempDir()
	store, b := harness(t, root)
	script := filepath.Join(root, "echoarg")
	out := filepath.Join(root, "out")
	mustWrite(t, script, "#!/bin/sh\nprintf '%s' \"$1\" > \"$2\"\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	arg := "hello world; rm -rf /"
	res := Wrap(context.Background(), WrapOptions{
		Boundary: b, Store: store, Argv: []string{script, arg, out},
		Stdout: io.Discard, Stderr: io.Discard,
	})
	if res.WrapperExit != 0 {
		t.Fatalf("%+v", res.Record)
	}
	if len(res.Record.Argv) != 3 || res.Record.Argv[1] != arg {
		t.Fatalf("argv %+v", res.Record.Argv)
	}
	body, err := os.ReadFile(out)
	if err != nil || string(body) != arg {
		t.Fatalf("%q %v", body, err)
	}
}

func TestWrapChildExitCode(t *testing.T) {
	root := t.TempDir()
	store, b := harness(t, root)
	res := Wrap(context.Background(), WrapOptions{
		Boundary: b, Store: store, Argv: []string{"false"},
		Stdout: io.Discard, Stderr: io.Discard,
	})
	if res.WrapperExit != 1 || res.Record.State != StateCompleted {
		t.Fatalf("exit %d state %s", res.WrapperExit, res.Record.State)
	}
	if res.Record.Process.ExitCode == nil || *res.Record.Process.ExitCode != 1 {
		t.Fatal("process.exit_code")
	}
	if res.Record.AgentUndo.ErrorCode != 0 {
		t.Fatal("not an internal error")
	}
}

func TestWrapMissingCommand(t *testing.T) {
	root := t.TempDir()
	store, b := harness(t, root)
	res := Wrap(context.Background(), WrapOptions{
		Boundary: b, Store: store, Argv: []string{"agent-undo-missing-bin"},
		Stdout: io.Discard, Stderr: io.Discard,
	})
	if res.WrapperExit != 3 || res.Record.State != StateFailed {
		t.Fatalf("%d %s", res.WrapperExit, res.Record.State)
	}
	if res.Record.CheckpointID == "" {
		t.Fatal("checkpoint should exist")
	}
	if !res.Record.Allows(OpUndo) {
		t.Fatal("undo after spawn fail")
	}
}

func TestWrapNestedLock(t *testing.T) {
	root := t.TempDir()
	store, b := harness(t, root)
	l, err := store.TryLock(b.Root())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Unlock()
	res := Wrap(context.Background(), WrapOptions{
		Boundary: b, Store: store, Argv: []string{"true"},
		Stdout: io.Discard, Stderr: io.Discard,
	})
	if res.WrapperExit != 3 || res.Record.AgentUndo.Error != NestedMessage() {
		t.Fatalf("%+v", res.Record)
	}
	if res.Record.CheckpointID != "" {
		t.Fatal("no checkpoint before lock")
	}
	if res.Record.Allows(OpUndo) {
		t.Fatal("undo not allowed")
	}
}

func TestWrapInterrupt(t *testing.T) {
	root := t.TempDir()
	store, b := harness(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan WrapResult, 1)
	go func() {
		done <- Wrap(ctx, WrapOptions{
			Boundary: b, Store: store, Argv: []string{"sleep", "30"},
			Stdout: io.Discard, Stderr: io.Discard, Grace: 40 * time.Millisecond,
		})
	}()
	waitState(t, store, StateRunning)
	cancel()
	res := <-done
	if res.WrapperExit != 130 || res.Record.State != StateInterrupted {
		t.Fatalf("exit %d state %s outcome %s err %s", res.WrapperExit, res.Record.State, res.Record.Outcome, res.Record.AgentUndo.Error)
	}
	if res.Record.FinalManifestID == "" {
		t.Fatal("final manifest after interrupt")
	}
}

func TestWrapSecondRunWhileHeld(t *testing.T) {
	root := t.TempDir()
	store, b := harness(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan WrapResult, 1)
	go func() {
		done <- Wrap(ctx, WrapOptions{
			Boundary: b, Store: store, Argv: []string{"sleep", "30"},
			Stdout: io.Discard, Stderr: io.Discard, Grace: 40 * time.Millisecond,
		})
	}()
	waitState(t, store, StateRunning)
	res2 := Wrap(context.Background(), WrapOptions{
		Boundary: b, Store: store, Argv: []string{"true"},
		Stdout: io.Discard, Stderr: io.Discard,
	})
	if res2.WrapperExit != 3 || res2.Record.AgentUndo.Error != NestedMessage() {
		t.Fatalf("%+v", res2.Record)
	}
	cancel()
	<-done
}

func TestWrapUndoAfterAgent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "old")
	store, b := harness(t, root)
	script := filepath.Join(root, "agent.sh")
	mustWrite(t, script, "#!/bin/sh\nprintf new > a.txt\n")
	_ = os.Chmod(script, 0o755)
	res := Wrap(context.Background(), WrapOptions{
		Boundary: b, Store: store, Argv: []string{script},
		Stdout: io.Discard, Stderr: io.Discard,
	})
	if res.WrapperExit != 0 {
		t.Fatal(res.Record.AgentUndo.Error)
	}
	rep := restore.Run(context.Background(), restore.Options{
		Boundary: b, Store: store, TargetID: res.Record.CheckpointID, Yes: true,
	})
	if rep.Outcome != restore.OutcomeSuccess {
		t.Fatalf("%s %v", rep.Outcome, rep.Err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(body) != "old" {
		t.Fatalf("%q", body)
	}
}

func TestRestoreRejectedWhileRunLockHeld(t *testing.T) {
	root := t.TempDir()
	store, b := harness(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan WrapResult, 1)
	go func() {
		done <- Wrap(ctx, WrapOptions{
			Boundary: b, Store: store, Argv: []string{"sleep", "30"},
			Stdout: io.Discard, Stderr: io.Discard, Grace: 40 * time.Millisecond,
		})
	}()
	id := waitState(t, store, StateRunning)
	rec, err := LoadRecord(store, id)
	if err != nil {
		t.Fatal(err)
	}
	rep := restore.Run(context.Background(), restore.Options{
		Boundary: b, Store: store, TargetID: rec.CheckpointID, Yes: true,
	})
	if rep.Err == nil || !strings.Contains(rep.Err.Error(), "repository locked") {
		t.Fatalf("%v", rep.Err)
	}
	cancel()
	<-done
}

func TestWrapGitCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git")
	}
	root := t.TempDir()
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(string(out), "Operation not permitted") {
				t.Skip(string(out))
			}
			t.Fatalf("%s %v", out, err)
		}
	}
	git("init", "-q", "-b", "main", "--template="+t.TempDir())
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	git("add", "a.txt")
	git("commit", "-q", "-m", "init")
	store, b := harness(t, root)
	script := filepath.Join(root, "agent.sh")
	mustWrite(t, script, "#!/bin/sh\nprintf b > a.txt\ngit -c core.hooksPath=/dev/null add a.txt\ngit -c core.hooksPath=/dev/null commit -q -m agent\n")
	_ = os.Chmod(script, 0o755)
	res := Wrap(context.Background(), WrapOptions{
		Boundary: b, Store: store, Argv: []string{script},
		Stdout: io.Discard, Stderr: io.Discard,
	})
	if res.WrapperExit != 0 {
		t.Fatalf("%s", res.Record.AgentUndo.Error)
	}
	if !res.Record.GitBefore.Captured || res.Record.GitAfter.Head == res.Record.GitBefore.Head {
		t.Fatalf("git %+v %+v", res.Record.GitBefore, res.Record.GitAfter)
	}
}

func waitState(t *testing.T, store *storage.Store, want State) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		recs, err := ListRecords(store)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range recs {
			if r.State == want {
				return r.ID
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never reached %s", want)
	return ""
}

func harness(t *testing.T, root string) (*storage.Store, *security.Boundary) {
	t.Helper()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	return store, b
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

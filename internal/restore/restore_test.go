package restore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luxavr/agent-undo/adapters/git"
	"github.com/luxavr/agent-undo/internal/checkpoint"
	"github.com/luxavr/agent-undo/internal/security"
	"github.com/luxavr/agent-undo/internal/storage"
	"github.com/luxavr/agent-undo/internal/verify"
)

func TestPipelineOrder(t *testing.T) {
	t.Parallel()
	want := []Stage{
		StageLock, StageLoad, StageValidate, StagePlan,
		StageRecoveryCheckpoint, StageApply, StageVerify, StageReport,
	}
	if len(Pipeline) != len(want) {
		t.Fatalf("got %v", Pipeline)
	}
	for i := range want {
		if Pipeline[i] != want[i] {
			t.Fatalf("got %v", Pipeline)
		}
	}
}

func TestNonGitInvariant(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.txt"), "alpha")
	mustWrite(t, filepath.Join(root, "gone.txt"), "bye")
	mustWrite(t, filepath.Join(root, "sub", "nested.txt"), "n")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)

	mustWrite(t, filepath.Join(root, "keep.txt"), "mutated")
	mustWrite(t, filepath.Join(root, "new.txt"), "created")
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "sub", "nested.txt"), filepath.Join(root, "renamed.txt")); err != nil {
		t.Fatal(err)
	}

	rep := mustUndo(t, store, b, cp.ID)
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v", rep.Outcome, rep.Err)
	}
	assertFile(t, filepath.Join(root, "keep.txt"), "alpha")
	assertFile(t, filepath.Join(root, "gone.txt"), "bye")
	assertFile(t, filepath.Join(root, "sub", "nested.txt"), "n")
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("created file should be deleted")
	}
	if _, err := os.Stat(filepath.Join(root, "renamed.txt")); !os.IsNotExist(err) {
		t.Fatal("rename leftover should be deleted")
	}
}

func TestNonGitSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "t"), "x")
	if err := os.Symlink("t", filepath.Join(root, "l")); err != nil {
		t.Fatal(err)
	}
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	if err := os.Remove(filepath.Join(root, "l")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "l")); err != nil {
		t.Fatal(err)
	}
	rep := mustUndo(t, store, b, cp.ID)
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v", rep.Outcome, rep.Err)
	}
	got, err := os.Readlink(filepath.Join(root, "l"))
	if err != nil || got != "t" {
		t.Fatalf("link %q %v", got, err)
	}
}

func TestGitDirtyAndUntracked(t *testing.T) {
	t.Parallel()
	root := gitRepo(t)
	mustWrite(t, filepath.Join(root, "tracked.txt"), "v1")
	gitRun(t, root, "add", "tracked.txt")
	gitRun(t, root, "commit", "-q", "-m", "init")
	mustWrite(t, filepath.Join(root, "tracked.txt"), "dirty")
	mustWrite(t, filepath.Join(root, "untracked.txt"), "u")
	mustWrite(t, filepath.Join(root, ".gitignore"), ".env.local\n")
	mustWrite(t, filepath.Join(root, ".env.local"), "SECRET=1\n")
	gitRun(t, root, "add", ".gitignore")
	gitRun(t, root, "commit", "-q", "-m", "ignore")

	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	if !cp.Git.Captured {
		t.Fatal("git should be captured")
	}
	mustWrite(t, filepath.Join(root, "tracked.txt"), "agent")
	mustWrite(t, filepath.Join(root, "agent-only.txt"), "nope")
	if err := os.Remove(filepath.Join(root, ".env.local")); err != nil {
		t.Fatal(err)
	}

	rep := mustUndo(t, store, b, cp.ID)
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v verify=%v", rep.Outcome, rep.Err, rep.Verify.Mismatches)
	}
	assertFile(t, filepath.Join(root, "tracked.txt"), "dirty")
	assertFile(t, filepath.Join(root, "untracked.txt"), "u")
	assertFile(t, filepath.Join(root, ".env.local"), "SECRET=1\n")
	if _, err := os.Stat(filepath.Join(root, "agent-only.txt")); !os.IsNotExist(err) {
		t.Fatal("agent file should be gone")
	}
}

func TestGitSessionCommitAndBranch(t *testing.T) {
	t.Parallel()
	root := gitRepo(t)
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-q", "-m", "init")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	captured := cp.Git.Head

	gitRun(t, root, "checkout", "-q", "-b", "agent-branch")
	mustWrite(t, filepath.Join(root, "a.txt"), "b")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-q", "-m", "agent")
	st, err := git.Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	sessionHEAD := st.Head

	rep := mustUndo(t, store, b, cp.ID)
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v %v", rep.Outcome, rep.Err, rep.Verify.Mismatches)
	}
	st2, err := git.Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if st2.Head != captured {
		t.Fatalf("HEAD %s want %s", st2.Head, captured)
	}
	if st2.Branch != cp.Git.Branch {
		t.Fatalf("branch %s want %s", st2.Branch, cp.Git.Branch)
	}
	ok, err := git.ObjectExists(context.Background(), root, sessionHEAD)
	if err != nil || !ok {
		t.Fatal("session commit must remain in the object database")
	}
	assertFile(t, filepath.Join(root, "a.txt"), "a")
}

func TestGitRewrittenHistoryStillRestores(t *testing.T) {
	t.Parallel()
	root := gitRepo(t)
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-q", "-m", "init")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	gitRun(t, root, "checkout", "--orphan", "orphan")
	mustWrite(t, filepath.Join(root, "a.txt"), "orphan")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-q", "-m", "orphan-root")
	rep := mustUndo(t, store, b, cp.ID)
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v %v", rep.Outcome, rep.Err, rep.Verify.Mismatches)
	}
	if !rep.Plan.Git.RewrittenHistory {
		t.Fatal("plan must flag rewritten history")
	}
	if len(rep.Plan.Git.SessionCommits) != 0 {
		t.Fatal("must not invent a linear commit list")
	}
	assertFile(t, filepath.Join(root, "a.txt"), "a")
}

func TestAbortWithoutYes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a"), "1")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	mustWrite(t, filepath.Join(root, "a"), "2")
	rep := Run(context.Background(), Options{Boundary: b, Store: store, TargetID: cp.ID})
	if rep.Outcome != OutcomeAborted {
		t.Fatalf("%s %v", rep.Outcome, rep.Err)
	}
	assertFile(t, filepath.Join(root, "a"), "2")
}

func TestConfirmNo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a"), "1")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	mustWrite(t, filepath.Join(root, "a"), "2")
	rep := Run(context.Background(), Options{
		Boundary: b, Store: store, TargetID: cp.ID,
		Confirm: func(string) (bool, error) { return false, nil },
	})
	if rep.Outcome != OutcomeAborted {
		t.Fatalf("%s", rep.Outcome)
	}
	assertFile(t, filepath.Join(root, "a"), "2")
}

func TestMissingSHAFailClosed(t *testing.T) {
	t.Parallel()
	root := gitRepo(t)
	mustWrite(t, filepath.Join(root, "a"), "1")
	gitRun(t, root, "add", "a")
	gitRun(t, root, "commit", "-q", "-m", "i")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	raw, err := store.ReadManifest(cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(raw), cp.Git.Head, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1)
	if err := store.WriteManifest(cp.ID, []byte(s)); err != nil {
		t.Fatal(err)
	}
	rep := Run(context.Background(), Options{Boundary: b, Store: store, TargetID: cp.ID, Yes: true})
	if rep.Outcome == OutcomeSuccess {
		t.Fatal("must fail closed")
	}
}

func TestCancelBeforeLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a"), "1")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	ctx, cancel := context.WithCancel(context.Background())
	rep := Run(ctx, Options{
		Boundary: b, Store: store, TargetID: cp.ID, Yes: true,
		After: func(p string) {
			if p == PointBeforeLock {
				cancel()
			}
		},
	})
	if rep.Outcome == OutcomeSuccess {
		t.Fatal(rep.Outcome)
	}
}

func TestCancelAfterRecoveryBeforeOverlay(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a"), "1")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	mustWrite(t, filepath.Join(root, "a"), "2")
	ctx, cancel := context.WithCancel(context.Background())
	rep := Run(ctx, Options{
		Boundary: b, Store: store, TargetID: cp.ID, Yes: true,
		After: func(p string) {
			if p == PointBeforeOverlay {
				cancel()
			}
		},
	})
	if rep.Outcome == OutcomeSuccess {
		t.Fatal(rep.Outcome)
	}
	if rep.RecoveryID == "" {
		t.Fatal("recovery checkpoint must exist before overlay")
	}
	assertFile(t, filepath.Join(root, "a"), "2")
}

func TestConcurrentLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a"), "1")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	l, err := store.TryLock(b.Root())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Unlock()
	rep := Run(context.Background(), Options{Boundary: b, Store: store, TargetID: cp.ID, Yes: true})
	if rep.Outcome == OutcomeSuccess {
		t.Fatal("must fail closed on lock")
	}
}

func TestFileReplacedByNonEmptyDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "slot"), "file")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	if err := os.Remove(filepath.Join(root, "slot")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "slot", "nested.txt"), "agent")
	rep := mustUndo(t, store, b, cp.ID)
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v %v", rep.Outcome, rep.Err, rep.Verify.Mismatches)
	}
	assertFile(t, filepath.Join(root, "slot"), "file")
	fi, err := os.Lstat(filepath.Join(root, "slot"))
	if err != nil || fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("slot must be a regular file: %v %+v", err, fi)
	}
}

func TestFileReplacedByDirWithUncapturedChild(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "slot"), "file")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	if err := os.Remove(filepath.Join(root, "slot")); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", 1048576+1)
	mustWrite(t, filepath.Join(root, "slot", "huge.bin"), big)
	rep := mustUndo(t, store, b, cp.ID)
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v %v", rep.Outcome, rep.Err, rep.Verify.Mismatches)
	}
	assertFile(t, filepath.Join(root, "slot"), "file")
}

func TestOverlayUnlinksSymlinkWithoutFollowing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "slot"), "file")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	mustWrite(t, outside, "keep-me")
	if err := os.Remove(filepath.Join(root, "slot")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "slot"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "slot", "link")); err != nil {
		t.Fatal(err)
	}
	rep := mustUndo(t, store, b, cp.ID)
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v %v", rep.Outcome, rep.Err, rep.Verify.Mismatches)
	}
	assertFile(t, filepath.Join(root, "slot"), "file")
	assertFile(t, outside, "keep-me")
}

func TestAgentCreatedDirRemoved(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.txt"), "k")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	mustWrite(t, filepath.Join(root, "newdir", "a.txt"), "x")
	rep := mustUndo(t, store, b, cp.ID)
	if rep.Outcome != OutcomeSuccess {
		t.Fatalf("%s %v %v", rep.Outcome, rep.Err, rep.Verify.Mismatches)
	}
	if _, err := os.Stat(filepath.Join(root, "newdir")); !os.IsNotExist(err) {
		t.Fatal("agent-created directory must be gone")
	}
	assertFile(t, filepath.Join(root, "keep.txt"), "k")
}

func TestVerifyCLIContract(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a"), "1")
	store, b := harness(t, root)
	cp := mustCreate(t, store, b)
	vr, err := verify.Workspace(context.Background(), cp, b)
	if err != nil || vr.Status != verify.StatusSuccess {
		t.Fatalf("%v %v", vr, err)
	}
	mustWrite(t, filepath.Join(root, "a"), "2")
	vr, err = verify.Workspace(context.Background(), cp, b)
	if err != nil || vr.Status != verify.StatusFailed {
		t.Fatalf("%v %v", vr, err)
	}
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

func mustCreate(t *testing.T, store *storage.Store, b *security.Boundary) *checkpoint.Manifest {
	t.Helper()
	m, err := checkpoint.Create(context.Background(), checkpoint.Options{Boundary: b, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustUndo(t *testing.T, store *storage.Store, b *security.Boundary, id string) *Report {
	t.Helper()
	rep := Run(context.Background(), Options{Boundary: b, Store: store, TargetID: id, Yes: true})
	if rep.Outcome != OutcomeSuccess {
		t.Helper()
	}
	return rep
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

func assertFile(t *testing.T, path, body string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != body {
		t.Fatalf("%s: got %q want %q", path, b, body)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main", "--template="+t.TempDir())
	return root
}

func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "Operation not permitted") {
			t.Skip(strings.TrimSpace(string(out)))
		}
		t.Fatalf("git %v: %s %v", args, out, err)
	}
}

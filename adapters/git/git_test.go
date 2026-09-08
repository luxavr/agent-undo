package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectNoGit(t *testing.T) {
	t.Parallel()
	st, err := Inspect(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if st.Present || st.Captured {
		t.Fatalf("%+v", st)
	}
}

func TestInspectFakeHEAD(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte(sha+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Present || !st.Captured || st.Head != sha || st.Branch != "main" || st.Detached {
		t.Fatalf("%+v", st)
	}
}

func TestInspectUnborn(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Unborn || st.Branch != "main" || st.Head != "" {
		t.Fatalf("%+v", st)
	}
}

func TestInspectRealRepo(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(string(out), "Operation not permitted") {
				t.Skip(strings.TrimSpace(string(out)))
			}
			t.Fatalf("git %v: %s %v", args, out, err)
		}
	}
	run("init", "-q", "--template="+t.TempDir())
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-q", "-m", "init")
	st, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Captured || len(st.Head) < 40 {
		t.Fatalf("%+v", st)
	}
	if _, ok := st.Tracked["a.txt"]; !ok {
		t.Fatalf("tracked=%v", st.Tracked)
	}
}

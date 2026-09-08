package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luxavr/agent-undo/internal/security"
	"github.com/luxavr/agent-undo/internal/storage"
)

func TestInspectNonGitReady(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	rep := Inspect(context.Background(), Options{Root: root, Home: home, GOOS: "darwin"})
	if rep.Status != StatusReady {
		t.Fatalf("%s\n%s", rep.Status, Render(rep))
	}
	out := Render(rep)
	if !strings.Contains(out, "git repository not present") {
		t.Fatal(out)
	}
	if strings.Contains(out, "⚠ Git index") || strings.Contains(out, "⚠ external side effects") {
		t.Fatal("standing limitations must not be warnings:\n", out)
	}
	if !strings.Contains(out, "- Git index/staging fidelity is not captured") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "- Cursor/editor-native attachment is not supported in v0.1") {
		t.Fatal(out)
	}
}

func TestInspectCheckingAbsolutePath(t *testing.T) {
	root := t.TempDir()
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	rep := Inspect(context.Background(), Options{Root: root, Home: t.TempDir(), GOOS: "darwin"})
	out := Render(rep)
	want := "checking: " + b.Root()
	if !strings.Contains(out, want) {
		t.Fatalf("missing %q in\n%s", want, out)
	}
	checkAt := strings.Index(out, "checking:")
	envAt := strings.Index(out, "Environment")
	if checkAt < 0 || envAt < 0 || checkAt > envAt {
		t.Fatalf("checking must appear above Environment:\n%s", out)
	}
}

func TestInspectLinuxLabel(t *testing.T) {
	rep := Inspect(context.Background(), Options{Root: t.TempDir(), Home: t.TempDir(), GOOS: "linux"})
	if rep.Status != StatusReady {
		t.Fatal(Render(rep))
	}
	if !strings.Contains(Render(rep), "Supported OS: Linux") {
		t.Fatal(Render(rep))
	}
}

func TestInspectUnsupportedOS(t *testing.T) {
	rep := Inspect(context.Background(), Options{Root: t.TempDir(), Home: t.TempDir(), GOOS: "windows"})
	if rep.Status != StatusNotReady {
		t.Fatalf("%s", rep.Status)
	}
	if !strings.Contains(Render(rep), "unsupported OS: Windows") {
		t.Fatal(Render(rep))
	}
}

func TestInspectInvalidBoundary(t *testing.T) {
	file := filepath.Join(t.TempDir(), "notdir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := Inspect(context.Background(), Options{Root: file, Home: t.TempDir(), GOOS: "darwin"})
	if rep.Status != StatusNotReady {
		t.Fatalf("%s\n%s", rep.Status, Render(rep))
	}
	if !strings.Contains(Render(rep), "checking: ") {
		t.Fatalf("invalid boundary must still name the path:\n%s", Render(rep))
	}
}

func TestInspectUnwritableStore(t *testing.T) {
	home := t.TempDir()
	if _, err := storage.Open(home); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o755) })
	rep := Inspect(context.Background(), Options{Root: t.TempDir(), Home: home, GOOS: "darwin"})
	if rep.Status != StatusNotReady {
		t.Fatalf("%s\n%s", rep.Status, Render(rep))
	}
	if !strings.Contains(Render(rep), "storage unwritable") && !strings.Contains(Render(rep), "lock cannot") {
		t.Fatal(Render(rep))
	}
}

func TestInspectCorruptManifest(t *testing.T) {
	home := t.TempDir()
	st, err := storage.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Home, "manifests", "bad.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := Inspect(context.Background(), Options{Root: t.TempDir(), Home: home, GOOS: "darwin"})
	if rep.Status != StatusNotReady {
		t.Fatalf("%s\n%s", rep.Status, Render(rep))
	}
	if !strings.Contains(Render(rep), "checkpoint store corrupted") {
		t.Fatal(Render(rep))
	}
}

func TestInspectLockHeld(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	st, err := storage.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := st.TryLock(b.Root())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Unlock()
	rep := Inspect(context.Background(), Options{Root: root, Home: home, GOOS: "darwin"})
	if rep.Status != StatusReadyWithWarnings {
		t.Fatalf("%s\n%s", rep.Status, Render(rep))
	}
	out := Render(rep)
	if !strings.Contains(out, "repository lock currently held") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "pid:") {
		t.Fatal(out)
	}
}

func TestInspectLockFileNotHeld(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	st, err := storage.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ProbeLock(b.Root()); err != nil {
		t.Fatal(err)
	}
	rep := Inspect(context.Background(), Options{Root: root, Home: home, GOOS: "darwin"})
	if rep.Status != StatusReady {
		t.Fatalf("leftover lock file is not held:\n%s", Render(rep))
	}
}

func TestInspectExclusionClasses(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "secret.js"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := Inspect(context.Background(), Options{Root: root, Home: t.TempDir(), GOOS: "darwin"})
	if rep.Status != StatusReadyWithWarnings {
		t.Fatalf("%s\n%s", rep.Status, Render(rep))
	}
	out := Render(rep)
	if !strings.Contains(out, "dependency tree detected: node_modules") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "build output detected: dist") {
		t.Fatal(out)
	}
	if strings.Contains(out, "secret.js") {
		t.Fatal("must not dump paths")
	}
}

func TestInspectExternalSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	rep := Inspect(context.Background(), Options{Root: root, Home: t.TempDir(), GOOS: "darwin"})
	if rep.Status != StatusReadyWithWarnings {
		t.Fatalf("%s\n%s", rep.Status, Render(rep))
	}
	if !strings.Contains(Render(rep), "external symlink target detected") {
		t.Fatal(Render(rep))
	}
	if strings.Contains(Render(rep), "/etc/passwd") {
		t.Fatal("must not print targets as an inventory")
	}
}

func TestInspectGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git")
	}
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", "-b", "main", "--template="+t.TempDir(), root)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "Operation not permitted") {
			t.Skip(string(out))
		}
		t.Fatalf("%s %v", out, err)
	}
	rep := Inspect(context.Background(), Options{Root: root, Home: t.TempDir(), GOOS: "darwin"})
	if rep.Status != StatusReady {
		t.Fatalf(".git must not warn:\n%s", Render(rep))
	}
	out := Render(rep)
	if !strings.Contains(out, "git detected") || !strings.Contains(out, "HEAD readable") {
		t.Fatal(out)
	}
}

func TestInspectMissingGitBinaryOnRepo(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	rep := Inspect(context.Background(), Options{Root: root, Home: t.TempDir(), GOOS: "darwin"})
	if rep.Status != StatusReadyWithWarnings {
		t.Fatalf("%s\n%s", rep.Status, Render(rep))
	}
	if !strings.Contains(Render(rep), "git executable not on PATH") {
		t.Fatal(Render(rep))
	}
}

func TestInspectDoesNotLeaveLockHeld(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	_ = Inspect(context.Background(), Options{Root: root, Home: home, GOOS: "darwin"})
	st, err := storage.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := st.TryLock(b.Root())
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Unlock()
}

func TestRenderDeterministic(t *testing.T) {
	t.Parallel()
	rep := Inspect(context.Background(), Options{Root: t.TempDir(), Home: t.TempDir(), GOOS: "darwin"})
	if Render(rep) != Render(rep) {
		t.Fatal("render must be deterministic")
	}
}

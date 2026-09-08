package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idlfirhan/agent-undo/internal/ignore"
	"github.com/idlfirhan/agent-undo/internal/security"
	"github.com/idlfirhan/agent-undo/internal/storage"
)

func TestCheckpointNoGit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "a.go"), "package a\n")
	mustWrite(t, filepath.Join(root, ".env.local"), "SECRET=1\n")
	mustWrite(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "nope\n")
	mustWrite(t, filepath.Join(root, "empty", ".keep"), "")
	if err := os.Remove(filepath.Join(root, "empty", ".keep")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.go", filepath.Join(root, "src", "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	m := create(t, root)
	if m.Git.Captured {
		t.Fatal("git should not be captured")
	}
	paths := map[string]File{}
	for _, f := range m.Files {
		paths[f.Path] = f
	}
	if _, ok := paths["src/a.go"]; !ok {
		t.Fatal("missing src/a.go")
	}
	if paths["src/link"].Kind != KindSymlink || paths["src/link"].LinkTarget != "a.go" {
		t.Fatalf("link %+v", paths["src/link"])
	}
	if paths["escape"].LinkTarget != "/etc/passwd" {
		t.Fatalf("escape %+v", paths["escape"])
	}
	if _, ok := paths["node_modules"]; ok {
		t.Fatal("node_modules tree must be skipped")
	}
	if _, ok := paths["node_modules/pkg/index.js"]; ok {
		t.Fatal("node_modules contents must be skipped")
	}
	if _, ok := paths[".env.local"]; !ok {
		t.Fatal(".env.local must be captured")
	}
	if _, ok := paths["empty"]; !ok {
		t.Fatal("empty dir must be captured")
	}
	if !hasClass(m, ignore.ClassNodeModules) {
		t.Fatalf("exclusions %+v", m.Exclusions)
	}
	if !hasClass(m, ignore.ClassExternal) {
		t.Fatalf("expected symlink.external %+v", m.Exclusions)
	}
	sum := sha256.Sum256([]byte("package a\n"))
	if paths["src/a.go"].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash %s", paths["src/a.go"].SHA256)
	}
}

func TestOversizeUntrackedSkipped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	big := strings.Repeat("x", int(ignore.MaxUntrackedBytes)+1)
	mustWrite(t, filepath.Join(root, "blob.bin"), big)
	mustWrite(t, filepath.Join(root, "ok.bin"), "tiny")
	m := create(t, root)
	if find(m, "blob.bin") != nil {
		t.Fatal("oversize untracked must be skipped")
	}
	if find(m, "ok.bin") == nil {
		t.Fatal("small file missing")
	}
	if !hasClass(m, ignore.ClassOversize) {
		t.Fatal("oversize class")
	}
}

func TestTrackedOversizeCaptured(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	big := strings.Repeat("y", int(ignore.MaxUntrackedBytes)+1)
	mustWrite(t, filepath.Join(root, "fat.bin"), big)
	mustWrite(t, filepath.Join(root, ".gitignore"), ".env.local\n")
	mustWrite(t, filepath.Join(root, ".env.local"), "K=V\n")
	git := func(args ...string) {
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
	git("init", "-q", "--template="+t.TempDir())
	git("add", "fat.bin", ".gitignore")
	git("commit", "-q", "-m", "init")
	m := create(t, root)
	if !m.Git.Captured || m.Git.Head == "" {
		t.Fatalf("git %+v", m.Git)
	}
	if find(m, "fat.bin") == nil {
		t.Fatal("tracked oversize must be captured")
	}
	if find(m, ".env.local") == nil {
		t.Fatal("small ignored env must be captured")
	}
}

func TestPathTraversalNotCaptured(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	mustWrite(t, outside, "no")
	t.Cleanup(func() { _ = os.Remove(outside) })
	m := create(t, root)
	for _, f := range m.Files {
		if strings.Contains(f.Path, "..") {
			t.Fatalf("escaped path %q", f.Path)
		}
	}
}

func TestManifestJSONRoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a"), "b")
	store, b := setup(t, root)
	m := createOpts(t, store, b)
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var again Manifest
	if err := json.Unmarshal(raw, &again); err != nil {
		t.Fatal(err)
	}
	if again.ID != m.ID || len(again.Files) != len(m.Files) {
		t.Fatalf("%+v", again)
	}
}

func FuzzManifestUnmarshal(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":1,"id":"cp_x","kind":"session","repoRoot":"/r","files":[]}`))
	f.Add([]byte(`{`))
	f.Add([]byte(`{"schemaVersion":99}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		var m Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return
		}
		if m.SchemaVersion == schemaVersion && m.ID != "" {
			_ = m.Kind
		}
	})
}

func create(t *testing.T, root string) *Manifest {
	t.Helper()
	store, b := setup(t, root)
	return createOpts(t, store, b)
}

func setup(t *testing.T, root string) (*storage.Store, *security.Boundary) {
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

func createOpts(t *testing.T, store *storage.Store, b *security.Boundary) *Manifest {
	t.Helper()
	m, err := Create(context.Background(), Options{Boundary: b, Store: store, Kind: KindSession})
	if err != nil {
		t.Fatal(err)
	}
	return m
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

func find(m *Manifest, path string) *File {
	for i := range m.Files {
		if m.Files[i].Path == path {
			return &m.Files[i]
		}
	}
	return nil
}

func hasClass(m *Manifest, class string) bool {
	for _, e := range m.Exclusions {
		if e.Class == class && e.Count > 0 {
			return true
		}
	}
	return false
}

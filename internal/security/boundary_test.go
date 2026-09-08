package security

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelInside(t *testing.T) {
	t.Parallel()
	rel, err := Rel("/repo", "/repo/src/a.go")
	if err != nil || rel != "src/a.go" {
		t.Fatalf("got %q %v", rel, err)
	}
}

func TestRelCleansDotDot(t *testing.T) {
	t.Parallel()
	rel, err := Rel("/repo", "/repo/a/../b")
	if err != nil || rel != "b" {
		t.Fatalf("got %q %v", rel, err)
	}
}

func TestRelOutside(t *testing.T) {
	t.Parallel()
	cases := [][2]string{
		{"/repo", "/etc/passwd"},
		{"/repo", "/repo/../etc/passwd"},
		{"/repo", "/repo/a/../../etc/passwd"},
		{"/repo", "/"},
	}
	for _, c := range cases {
		_, err := Rel(c[0], c[1])
		if !errors.Is(err, ErrOutside) && err == nil {
			t.Fatalf("Rel(%q,%q) err=%v", c[0], c[1], err)
		}
		if err == nil {
			t.Fatalf("Rel(%q,%q) should fail", c[0], c[1])
		}
	}
}

func TestRelRootRejected(t *testing.T) {
	t.Parallel()
	if _, err := Rel("/repo", "/repo"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRelNUL(t *testing.T) {
	t.Parallel()
	if _, err := Rel("/repo", "/repo/a\x00b"); err == nil {
		t.Fatal("expected NUL error")
	}
}

func TestBoundaryJoinAndSymlinkLexical(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	b, err := NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	full, err := b.JoinRel("src/a.go")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(b.Root(), "src", "a.go")
	if full != want {
		t.Fatalf("got %q want %q", full, want)
	}
	if _, err := b.JoinRel("../etc/passwd"); err == nil {
		t.Fatal("expected escape")
	}
	if _, err := b.JoinRel("/etc/passwd"); err == nil {
		t.Fatal("expected abs reject")
	}
	if _, err := b.JoinRel("a/../../etc/passwd"); err == nil {
		t.Fatal("expected escape")
	}

	link := filepath.Join(root, "escape")
	if err := os.Symlink("/etc/passwd", link); err != nil {
		t.Fatal(err)
	}
	rel, err := b.RelPath(link)
	if err != nil || rel != "escape" {
		t.Fatalf("symlink node must be inside lexically: %q %v", rel, err)
	}
	if !b.TargetEscapes("escape", "/etc/passwd") {
		t.Fatal("absolute outside target must escape")
	}
	if !b.TargetEscapes("sub/link", "../../etc/passwd") {
		t.Fatal("relative outside target must escape")
	}
	if b.TargetEscapes("sub/link", "nearby") {
		t.Fatal("in-repo relative target must not escape")
	}
	if b.TargetEscapes("link", "in-repo") {
		t.Fatal("root-level relative target must not escape")
	}
}

func FuzzRel(f *testing.F) {
	f.Add("/repo", "/repo/a")
	f.Add("/repo", "/repo/../etc/passwd")
	f.Add("/var/repo", "/var/repo/a/../../etc")
	f.Add("/repo", "/repo")
	f.Add("/repo", "/repo/a\x00b")
	f.Fuzz(func(t *testing.T, root, candidate string) {
		rel, err := Rel(root, candidate)
		if err != nil {
			return
		}
		if rel == "" || rel == "." || hasDotDotComponent(rel) {
			t.Fatalf("accepted unsafe rel=%q root=%q cand=%q", rel, root, candidate)
		}
		joined := filepath.Join(filepath.Clean(root), filepath.FromSlash(rel))
		again, err := Rel(root, joined)
		if err != nil || again != rel {
			t.Fatalf("roundtrip rel=%q again=%q err=%v", rel, again, err)
		}
	})
}

func hasDotDotComponent(rel string) bool {
	for _, p := range strings.Split(rel, "/") {
		if p == ".." {
			return true
		}
	}
	return false
}

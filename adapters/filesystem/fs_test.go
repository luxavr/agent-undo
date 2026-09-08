package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/luxavr/agent-undo/internal/security"
)

func TestOpenRejectsEscape(t *testing.T) {
	t.Parallel()
	b, err := security.NewBoundary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(b, "../secret"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestReadlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Symlink("target", filepath.Join(root, "l")); err != nil {
		t.Fatal(err)
	}
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Readlink(b, "l")
	if err != nil || got != "target" {
		t.Fatalf("%q %v", got, err)
	}
}

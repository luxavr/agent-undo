package verify

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/luxavr/agent-undo/internal/checkpoint"
	"github.com/luxavr/agent-undo/internal/security"
	"github.com/luxavr/agent-undo/internal/storage"
)

func TestWorkspaceMatchAndMismatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Create(context.Background(), checkpoint.Options{Boundary: b, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	r, err := Workspace(context.Background(), cp, b)
	if err != nil || r.Status != StatusSuccess {
		t.Fatalf("%+v %v", r, err)
	}
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err = Workspace(context.Background(), cp, b)
	if err != nil || r.Status != StatusFailed {
		t.Fatalf("%+v %v", r, err)
	}
}

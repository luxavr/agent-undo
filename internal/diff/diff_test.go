package diff

import (
	"strconv"
	"strings"
	"testing"

	"github.com/luxavr/agent-undo/internal/checkpoint"
)

func TestNilManifests(t *testing.T) {
	t.Parallel()
	if _, err := Compare(nil, &checkpoint.Manifest{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Compare(&checkpoint.Manifest{}, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestEmptyIdentical(t *testing.T) {
	t.Parallel()
	a := &checkpoint.Manifest{}
	b := &checkpoint.Manifest{}
	r, err := Compare(a, b)
	if err != nil || !r.Equal() || len(r.Entries) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestIdenticalFiles(t *testing.T) {
	t.Parallel()
	f := checkpoint.File{Path: "a.go", Kind: checkpoint.KindFile, Mode: "644", Size: 1, SHA256: "abc"}
	r, err := Compare(&checkpoint.Manifest{Files: []checkpoint.File{f}}, &checkpoint.Manifest{Files: []checkpoint.File{f}})
	if err != nil || !r.Equal() || r.Paths(Unchanged)[0] != "a.go" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestAddedDeletedModified(t *testing.T) {
	t.Parallel()
	a := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "gone.txt", Kind: checkpoint.KindFile, SHA256: "1", Size: 1, Mode: "644"},
		{Path: "stay.txt", Kind: checkpoint.KindFile, SHA256: "2", Size: 1, Mode: "644"},
		{Path: "edit.txt", Kind: checkpoint.KindFile, SHA256: "3", Size: 1, Mode: "644"},
	}}
	b := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "new.txt", Kind: checkpoint.KindFile, SHA256: "9", Size: 1, Mode: "644"},
		{Path: "stay.txt", Kind: checkpoint.KindFile, SHA256: "2", Size: 1, Mode: "644"},
		{Path: "edit.txt", Kind: checkpoint.KindFile, SHA256: "4", Size: 1, Mode: "644"},
	}}
	r, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Paths(Added); len(got) != 1 || got[0] != "new.txt" {
		t.Fatalf("added %v", got)
	}
	if got := r.Paths(Deleted); len(got) != 1 || got[0] != "gone.txt" {
		t.Fatalf("deleted %v", got)
	}
	if got := r.Paths(Modified); len(got) != 1 || got[0] != "edit.txt" {
		t.Fatalf("modified %v", got)
	}
	if r.Equal() {
		t.Fatal("should not equal")
	}
}

func TestTypeAndSymlinkChange(t *testing.T) {
	t.Parallel()
	a := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "x", Kind: checkpoint.KindFile, SHA256: "1", Size: 1, Mode: "644"},
		{Path: "l", Kind: checkpoint.KindSymlink, LinkTarget: "a", Mode: "777"},
	}}
	b := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "x", Kind: checkpoint.KindSymlink, LinkTarget: "a", Mode: "777"},
		{Path: "l", Kind: checkpoint.KindSymlink, LinkTarget: "b", Mode: "777"},
	}}
	r, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if r.Paths(TypeChanged)[0] != "x" {
		t.Fatalf("%v", r.Entries)
	}
	if r.Paths(SymlinkChanged)[0] != "l" {
		t.Fatalf("%v", r.Entries)
	}
}

func TestRenameIsDeleteAndAdd(t *testing.T) {
	t.Parallel()
	a := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "old", Kind: checkpoint.KindFile, SHA256: "same", Size: 1, Mode: "644"},
	}}
	b := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "new", Kind: checkpoint.KindFile, SHA256: "same", Size: 1, Mode: "644"},
	}}
	r, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if r.Count(Deleted) != 1 || r.Count(Added) != 1 || r.Count(Modified) != 0 {
		t.Fatalf("rename must be deleted+added: %+v", r.Entries)
	}
}

func TestStableOrder(t *testing.T) {
	t.Parallel()
	a := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "c", Kind: checkpoint.KindFile, SHA256: "1", Size: 1, Mode: "644"},
		{Path: "a", Kind: checkpoint.KindFile, SHA256: "1", Size: 1, Mode: "644"},
	}}
	b := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "b", Kind: checkpoint.KindFile, SHA256: "1", Size: 1, Mode: "644"},
		{Path: "a", Kind: checkpoint.KindFile, SHA256: "2", Size: 1, Mode: "644"},
	}}
	r, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, e := range r.Entries {
		paths = append(paths, e.Path)
	}
	if strings.Join(paths, ",") != "a,b,c" {
		t.Fatalf("order %v", paths)
	}
}

func TestEmptyPathSkipped(t *testing.T) {
	t.Parallel()
	a := &checkpoint.Manifest{Files: []checkpoint.File{{Path: "", Kind: checkpoint.KindFile, SHA256: "1"}}}
	b := &checkpoint.Manifest{Files: []checkpoint.File{{Path: "ok", Kind: checkpoint.KindFile, SHA256: "1", Size: 1, Mode: "644"}}}
	r, err := Compare(a, b)
	if err != nil || r.Count(Added) != 1 || r.Paths(Added)[0] != "ok" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestDuplicatePathFirstWins(t *testing.T) {
	t.Parallel()
	a := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "a", Kind: checkpoint.KindFile, SHA256: "1", Size: 1, Mode: "644"},
		{Path: "a", Kind: checkpoint.KindFile, SHA256: "2", Size: 1, Mode: "644"},
	}}
	b := &checkpoint.Manifest{Files: []checkpoint.File{
		{Path: "a", Kind: checkpoint.KindFile, SHA256: "1", Size: 1, Mode: "644"},
	}}
	r, err := Compare(a, b)
	if err != nil || !r.Equal() {
		t.Fatalf("first entry should win: %+v %v", r, err)
	}
}

func TestLargeManifest(t *testing.T) {
	t.Parallel()
	n := 2000
	files := make([]checkpoint.File, n)
	for i := 0; i < n; i++ {
		files[i] = checkpoint.File{Path: "f" + strconv.Itoa(i), Kind: checkpoint.KindFile, SHA256: "x", Size: 1, Mode: "644"}
	}
	r, err := Compare(&checkpoint.Manifest{Files: files}, &checkpoint.Manifest{Files: files})
	if err != nil || !r.Equal() || r.Count(Unchanged) != n {
		t.Fatalf("large %+v %v", r.Count(Unchanged), err)
	}
}

func TestDirModeChange(t *testing.T) {
	t.Parallel()
	a := &checkpoint.Manifest{Files: []checkpoint.File{{Path: "d", Kind: checkpoint.KindDir, Mode: "755"}}}
	b := &checkpoint.Manifest{Files: []checkpoint.File{{Path: "d", Kind: checkpoint.KindDir, Mode: "700"}}}
	r, err := Compare(a, b)
	if err != nil || r.Count(Modified) != 1 || r.Paths(Modified)[0] != "d" {
		t.Fatalf("%+v %v", r, err)
	}
}

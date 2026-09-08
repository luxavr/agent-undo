package diff

import (
	"fmt"
	"sort"

	"github.com/luxavr/agent-undo/internal/checkpoint"
)

// Class is a filesystem delta class. There is no renamed class.
type Class string

const (
	Added          Class = "added"
	Modified       Class = "modified"
	Deleted        Class = "deleted"
	TypeChanged    Class = "type-changed"
	SymlinkChanged Class = "symlink-changed"
	Unchanged      Class = "unchanged"
)

// Entry is one path in a deterministic Compare result.
type Entry struct {
	Path  string
	Class Class
}

// Result is a stable, mutation-free filesystem delta.
type Result struct {
	Entries []Entry
}

// Compare returns the filesystem delta from a (before / checkpoint) to b
// (after / live). Git metadata is not interpreted here.
func Compare(a, b *checkpoint.Manifest) (Result, error) {
	if a == nil || b == nil {
		return Result{}, fmt.Errorf("diff: nil manifest")
	}
	am := index(a.Files)
	bm := index(b.Files)
	paths := make([]string, 0, len(am)+len(bm))
	seen := map[string]struct{}{}
	for p := range am {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	for p := range bm {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var out Result
	for _, p := range paths {
		left, lok := am[p]
		right, rok := bm[p]
		switch {
		case !lok:
			out.Entries = append(out.Entries, Entry{Path: p, Class: Added})
		case !rok:
			out.Entries = append(out.Entries, Entry{Path: p, Class: Deleted})
		default:
			out.Entries = append(out.Entries, Entry{Path: p, Class: classify(left, right)})
		}
	}
	return out, nil
}

func classify(a, b checkpoint.File) Class {
	if a.Kind != b.Kind {
		return TypeChanged
	}
	switch a.Kind {
	case checkpoint.KindSymlink:
		if a.LinkTarget != b.LinkTarget || a.Mode != b.Mode {
			return SymlinkChanged
		}
		return Unchanged
	case checkpoint.KindFile:
		if a.SHA256 != b.SHA256 || a.Mode != b.Mode || a.Size != b.Size {
			return Modified
		}
		return Unchanged
	case checkpoint.KindDir:
		if a.Mode != b.Mode {
			return Modified
		}
		return Unchanged
	default:
		if a.SHA256 != b.SHA256 || a.Mode != b.Mode || a.LinkTarget != b.LinkTarget || a.Size != b.Size {
			return Modified
		}
		return Unchanged
	}
}

func index(files []checkpoint.File) map[string]checkpoint.File {
	m := make(map[string]checkpoint.File, len(files))
	for _, f := range files {
		if f.Path == "" {
			continue
		}
		if _, exists := m[f.Path]; exists {
			continue
		}
		m[f.Path] = f
	}
	return m
}

// Paths returns paths of class c in Compare order.
func (r Result) Paths(c Class) []string {
	var out []string
	for _, e := range r.Entries {
		if e.Class == c {
			out = append(out, e.Path)
		}
	}
	return out
}

// Count returns how many entries have class c.
func (r Result) Count(c Class) int {
	n := 0
	for _, e := range r.Entries {
		if e.Class == c {
			n++
		}
	}
	return n
}

// Equal is true when the only classes present are Unchanged (or the delta is empty).
func (r Result) Equal() bool {
	for _, e := range r.Entries {
		if e.Class != Unchanged {
			return false
		}
	}
	return true
}

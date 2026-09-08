package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrOutside is returned when a path is not inside the repository boundary.
var ErrOutside = errors.New("path outside repository boundary")

// Boundary is the canonical workspace root. All captured and restored paths
// must Rel to it.
type Boundary struct {
	root string
}

// NewBoundary resolves root to an existing directory and EvalSymlinks it so
// identity matches the real mount.
func NewBoundary(root string) (*Boundary, error) {
	if strings.ContainsRune(root, 0) {
		return nil, fmt.Errorf("path contains NUL")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs = filepath.Clean(abs)
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("boundary is not a directory: %s", abs)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Boundary{root: resolved}, nil
}

// Root returns the canonical absolute path.
func (b *Boundary) Root() string { return b.root }

// RelPath returns a slash-separated path inside the boundary. It is lexical:
// it does not follow the final component if that component is a symlink.
func (b *Boundary) RelPath(path string) (string, error) {
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("path contains NUL")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	parent := filepath.Dir(abs)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		resolvedParent = parent
	}
	return Rel(b.root, filepath.Join(resolvedParent, filepath.Base(abs)))
}

// Rel reports whether cleaned candidate is lexically inside cleaned root.
// Used by tests and fuzz without touching the filesystem. Paths should be
// absolute; relative paths are cleaned as-is.
func Rel(root, candidate string) (string, error) {
	if strings.ContainsRune(root, 0) || strings.ContainsRune(candidate, 0) {
		return "", fmt.Errorf("path contains NUL")
	}
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrOutside, candidate)
	}
	if rel == "." {
		return "", fmt.Errorf("refusing repository root as a file path")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %s", ErrOutside, candidate)
	}
	return filepath.ToSlash(rel), nil
}

// JoinRel joins a slash-separated manifest path onto the boundary. The result
// is still checked with Rel so ".." components cannot escape.
func (b *Boundary) JoinRel(rel string) (string, error) {
	if rel == "" || strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: absolute path", ErrOutside)
	}
	cleaned := filepath.Clean(filepath.FromSlash(rel))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrOutside, rel)
	}
	full := filepath.Join(b.root, cleaned)
	if _, err := Rel(b.root, full); err != nil {
		return "", err
	}
	return full, nil
}

// TargetEscapes reports whether a symlink target string, resolved lexically
// against the link's directory, lies outside the boundary. It does not read
// the target.
func (b *Boundary) TargetEscapes(linkRel, target string) bool {
	if strings.ContainsRune(target, 0) {
		return true
	}
	base := b.root
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(linkRel)))
	if dir != "." && dir != "" {
		joined, err := b.JoinRel(dir)
		if err != nil {
			return true
		}
		base = joined
	}
	var dest string
	if filepath.IsAbs(target) {
		dest = filepath.Clean(target)
	} else {
		dest = filepath.Clean(filepath.Join(base, target))
	}
	_, err := Rel(b.root, dest)
	return err != nil
}

package restore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/luxavr/agent-undo/internal/checkpoint"
	"github.com/luxavr/agent-undo/internal/security"
)

func overlay(ctx context.Context, opts Options, target *checkpoint.Manifest, deletePaths []string) error {
	b := opts.Boundary
	sorted := append([]string(nil), deletePaths...)
	sort.Slice(sorted, func(i, j int) bool {
		di, dj := strings.Count(sorted[i], "/"), strings.Count(sorted[j], "/")
		if di != dj {
			return di > dj
		}
		return sorted[i] > sorted[j]
	})
	for _, rel := range sorted {
		if err := opts.hit(ctx, PointDuringOverlay); err != nil {
			return err
		}
		if err := removeRel(b, rel); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %s: %w", rel, err)
		}
	}
	dirs, rest := splitKinds(target.Files)
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i].Path, "/") < strings.Count(dirs[j].Path, "/")
	})
	for _, f := range dirs {
		if err := opts.hit(ctx, PointDuringOverlay); err != nil {
			return err
		}
		if err := restoreDir(b, f); err != nil {
			return err
		}
	}
	for _, f := range rest {
		if err := opts.hit(ctx, PointDuringOverlay); err != nil {
			return err
		}
		switch f.Kind {
		case checkpoint.KindFile:
			if err := restoreFile(b, opts.Store, f); err != nil {
				return err
			}
		case checkpoint.KindSymlink:
			if err := restoreSymlink(b, f); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown kind %q for %s", f.Kind, f.Path)
		}
	}
	return nil
}

func splitKinds(files []checkpoint.File) (dirs, rest []checkpoint.File) {
	for _, f := range files {
		if f.Kind == checkpoint.KindDir {
			dirs = append(dirs, f)
		} else {
			rest = append(rest, f)
		}
	}
	return dirs, rest
}

func removeRel(b *security.Boundary, rel string) error {
	p, err := b.JoinRel(rel)
	if err != nil {
		return err
	}
	return removeInside(p)
}

func restoreDir(b *security.Boundary, f checkpoint.File) error {
	p, err := b.JoinRel(f.Path)
	if err != nil {
		return err
	}
	if err := removeInside(p); err != nil {
		return err
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		return err
	}
	return os.Chmod(p, fileMode(f.Mode))
}

func restoreFile(b *security.Boundary, store interface {
	GetBytes(string) ([]byte, error)
}, f checkpoint.File) error {
	p, err := b.JoinRel(f.Path)
	if err != nil {
		return err
	}
	data, err := store.GetBytes(f.SHA256)
	if err != nil {
		return fmt.Errorf("blob %s: %w", f.Path, err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := removeInside(p); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".au-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, fileMode(f.Mode)); err != nil {
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		return err
	}
	ok = true
	return nil
}

func restoreSymlink(b *security.Boundary, f checkpoint.File) error {
	p, err := b.JoinRel(f.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := removeInside(p); err != nil {
		return err
	}
	return os.Symlink(f.LinkTarget, p)
}

// removeInside deletes p after overlay has already JoinRel-canonicalized it
// inside the repository boundary. Directories use RemoveAll so a type-changed
// path with uncaptured children (oversize, etc.) cannot block restore.
// Symlinks are unlinked; their targets are not followed.
func removeInside(p string) error {
	fi, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return os.Remove(p)
	}
	if fi.IsDir() {
		return os.RemoveAll(p)
	}
	return os.Remove(p)
}

func fileMode(s string) os.FileMode {
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0o644
	}
	return os.FileMode(n)
}

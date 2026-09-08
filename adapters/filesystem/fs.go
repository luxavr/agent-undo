package filesystem

import (
	"os"

	"github.com/luxavr/agent-undo/internal/security"
)

// Open opens a regular file inside the boundary.
func Open(b *security.Boundary, rel string) (*os.File, error) {
	p, err := b.JoinRel(rel)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// Readlink returns the symlink target string. The link path must Rel inside
// the boundary; the target is not followed.
func Readlink(b *security.Boundary, rel string) (string, error) {
	p, err := b.JoinRel(rel)
	if err != nil {
		return "", err
	}
	return os.Readlink(p)
}

// Lstat stats a path inside the boundary without following the final symlink.
func Lstat(b *security.Boundary, rel string) (os.FileInfo, error) {
	p, err := b.JoinRel(rel)
	if err != nil {
		return nil, err
	}
	return os.Lstat(p)
}

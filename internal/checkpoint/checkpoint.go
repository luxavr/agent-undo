package checkpoint

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/idlfirhan/agent-undo/adapters/filesystem"
	"github.com/idlfirhan/agent-undo/adapters/git"
	"github.com/idlfirhan/agent-undo/internal/ignore"
	"github.com/idlfirhan/agent-undo/internal/security"
	"github.com/idlfirhan/agent-undo/internal/storage"
)

const schemaVersion = 1

const (
	KindSession  = "session"
	KindRecovery = "recovery"
	KindFinal    = "final"
	KindFile     = "file"
	KindSymlink  = "symlink"
	KindDir      = "dir"
)

// Manifest is the on-disk checkpoint record.
type Manifest struct {
	SchemaVersion int         `json:"schemaVersion"`
	ID            string      `json:"id"`
	Kind          string      `json:"kind"`
	RepoRoot      string      `json:"repoRoot"`
	CreatedAt     time.Time   `json:"createdAt"`
	Git           Git         `json:"git"`
	Files         []File      `json:"files"`
	Exclusions    []Exclusion `json:"exclusions"`
	Source        Source      `json:"source,omitempty"`
}

// Source is optional lineage on a recovery checkpoint. It is informational.
// APPLY and VERIFY must not depend on it.
type Source struct {
	Type         string `json:"type,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
	CheckpointID string `json:"checkpointId,omitempty"`
}

const (
	SourceUndo    = "undo"
	SourceRecover = "recover"
)

// Git is captured git identity. Captured=false means Git: not captured.
type Git struct {
	Captured bool   `json:"captured"`
	Head     string `json:"head,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Detached bool   `json:"detached,omitempty"`
	Unborn   bool   `json:"unborn,omitempty"`
}

// File is one captured path.
type File struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Mode       string `json:"mode"`
	Size       int64  `json:"size,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	LinkTarget string `json:"linkTarget,omitempty"`
}

// Exclusion is a skipped class with a count (not a path dump).
type Exclusion struct {
	Class string `json:"class"`
	Count int    `json:"count"`
}

// Options for Create.
type Options struct {
	Boundary *security.Boundary
	Store    *storage.Store
	Kind     string
	Source   Source
}

type blobPutter interface {
	PutFrom(r io.Reader) (string, int64, error)
}

type hashOnly struct{}

func (hashOnly) PutFrom(r io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// Scan walks the boundary and returns an in-memory manifest (no store write).
func Scan(ctx context.Context, b *security.Boundary) (*Manifest, error) {
	if b == nil {
		return nil, fmt.Errorf("checkpoint: boundary is required")
	}
	return assemble(ctx, b, KindSession, hashOnly{})
}

func assemble(ctx context.Context, b *security.Boundary, kind string, put blobPutter) (*Manifest, error) {
	gst, err := git.Inspect(ctx, b.Root())
	if err != nil {
		return nil, err
	}
	m := &Manifest{
		SchemaVersion: schemaVersion,
		ID:            newID(),
		Kind:          kind,
		RepoRoot:      b.Root(),
		CreatedAt:     time.Now().UTC().Truncate(time.Millisecond),
		Git: Git{
			Captured: gst.Captured,
			Head:     gst.Head,
			Branch:   gst.Branch,
			Detached: gst.Detached,
			Unborn:   gst.Unborn,
		},
	}
	counts := map[string]int{}
	err = filepath.WalkDir(b.Root(), func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == b.Root() {
			return nil
		}
		rel, err := b.RelPath(path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			dec := ignore.Dir(d.Name())
			if dec.SkipDir {
				counts[dec.Class]++
				return filepath.SkipDir
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			m.Files = append(m.Files, File{
				Path: rel,
				Kind: KindDir,
				Mode: modeString(info.Mode()),
			})
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			target, err := filesystem.Readlink(b, rel)
			if err != nil {
				return err
			}
			if b.TargetEscapes(rel, target) {
				counts[ignore.ClassExternal]++
			}
			m.Files = append(m.Files, File{
				Path:       rel,
				Kind:       KindSymlink,
				Mode:       modeString(mode),
				LinkTarget: target,
			})
			return nil
		}
		dec := ignore.File(mode, info.Size(), gitClass(gst, rel))
		if !dec.Capture {
			counts[dec.Class]++
			return nil
		}
		f, err := filesystem.Open(b, rel)
		if err != nil {
			return err
		}
		sum, n, err := put.PutFrom(f)
		_ = f.Close()
		if err != nil {
			return err
		}
		m.Files = append(m.Files, File{
			Path:   rel,
			Kind:   KindFile,
			Mode:   modeString(mode),
			Size:   n,
			SHA256: sum,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	for class, n := range counts {
		m.Exclusions = append(m.Exclusions, Exclusion{Class: class, Count: n})
	}
	sort.Slice(m.Exclusions, func(i, j int) bool { return m.Exclusions[i].Class < m.Exclusions[j].Class })
	return m, nil
}

// Create walks the boundary, writes objects, writes a verified manifest.
func Create(ctx context.Context, opts Options) (*Manifest, error) {
	if opts.Boundary == nil || opts.Store == nil {
		return nil, fmt.Errorf("checkpoint: boundary and store are required")
	}
	kind := opts.Kind
	if kind == "" {
		kind = KindSession
	}
	if kind != KindSession && kind != KindRecovery && kind != KindFinal {
		return nil, fmt.Errorf("checkpoint: unknown kind %q", kind)
	}
	m, err := assemble(ctx, opts.Boundary, kind, opts.Store)
	if err != nil {
		return nil, err
	}
	m.Source = opts.Source
	if err := VerifyComplete(opts.Store, m); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	if err := opts.Store.WriteManifest(m.ID, raw); err != nil {
		return nil, err
	}
	loaded, err := Load(opts.Store, m.ID)
	if err != nil {
		return nil, err
	}
	if err := VerifyComplete(opts.Store, loaded); err != nil {
		return nil, err
	}
	return loaded, nil
}

// Load reads a manifest from the store. Caller must VerifyComplete before restore.
func Load(store *storage.Store, id string) (*Manifest, error) {
	raw, err := store.ReadManifest(id)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("unsupported manifest schema %d", m.SchemaVersion)
	}
	return &m, nil
}

// VerifyComplete checks that every regular-file blob exists and hashes.
func VerifyComplete(store *storage.Store, m *Manifest) error {
	if m == nil {
		return fmt.Errorf("nil manifest")
	}
	for _, f := range m.Files {
		switch f.Kind {
		case KindFile:
			if f.SHA256 == "" {
				return fmt.Errorf("file %s missing sha256", f.Path)
			}
			b, err := store.GetBytes(f.SHA256)
			if err != nil {
				return fmt.Errorf("file %s: %w", f.Path, err)
			}
			if int64(len(b)) != f.Size {
				return fmt.Errorf("file %s size mismatch", f.Path)
			}
		case KindSymlink:
			if f.LinkTarget == "" && f.Path == "" {
				return fmt.Errorf("invalid symlink entry")
			}
		case KindDir:
		default:
			return fmt.Errorf("unknown kind %q for %s", f.Kind, f.Path)
		}
	}
	return nil
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "cp_" + hex.EncodeToString(b[:])
}

func gitClass(st git.State, rel string) ignore.GitClass {
	if !st.Present {
		return ignore.GitUnknown
	}
	if _, ok := st.Tracked[rel]; ok {
		return ignore.GitTracked
	}
	if _, ok := st.Ignored[rel]; ok {
		return ignore.GitIgnored
	}
	return ignore.GitUntracked
}

func modeString(mode fs.FileMode) string {
	return strconv.FormatUint(uint64(mode.Perm()), 8)
}

package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/luxavr/agent-undo/adapters/git"
	"github.com/luxavr/agent-undo/internal/ignore"
	"github.com/luxavr/agent-undo/internal/security"
	"github.com/luxavr/agent-undo/internal/storage"
)

const (
	// MaxWalkDepth is the maximum directory depth from the boundary (root = 0).
	MaxWalkDepth = 3
	// MaxWalkVisit is the maximum number of directory entries inspected.
	MaxWalkVisit = 4096
	// MaxManifestSample is how many manifest files are parsed for corruption.
	MaxManifestSample = 32
	supportedSchema   = 1
)

// Status is CLI and repository readiness. Standing limitations do not set it.
type Status string

const (
	StatusReady             Status = "READY"
	StatusReadyWithWarnings Status = "READY WITH WARNINGS"
	StatusNotReady          Status = "NOT READY"
)

// Level is a report line kind. Notes do not affect Status.
type Level int

const (
	LevelOK Level = iota
	LevelNote
	LevelWarn
	LevelFail
)

// Line is one diagnostic line.
type Line struct {
	Level Level
	Text  string
	Extra string
}

// Section is a labeled group of lines.
type Section struct {
	Title string
	Lines []Line
}

// Report is a deterministic doctor result. Render is a pure projection.
type Report struct {
	Status   Status
	Checking string
	Sections []Section
}

// Options for Inspect. GOOS may be overridden in tests.
type Options struct {
	Root     string
	Home     string
	UserHome string
	GOOS     string
}

// Inspect diagnoses Agent Undo prerequisites. It does not repair, crawl for
// hashes, or walk excluded trees. The lock probe acquires and immediately
// releases; it does not steal a held lock or delete lock files.
func Inspect(ctx context.Context, opts Options) Report {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	var env, repo, storeSec, fsSec, gitSec, support Section
	env.Title = "Environment"
	repo.Title = "Repository"
	storeSec.Title = "Checkpoint"
	fsSec.Title = "Filesystem"
	gitSec.Title = "Git"
	support.Title = "Support"

	env.Lines = append(env.Lines, Line{Level: LevelOK, Text: "Agent Undo CLI available"})
	osLabel, osOK := supportedOS(goos)
	if osOK {
		env.Lines = append(env.Lines, Line{Level: LevelOK, Text: "Supported OS: " + osLabel})
	} else {
		env.Lines = append(env.Lines, Line{Level: LevelFail, Text: "unsupported OS: " + osLabel})
	}
	env.Lines = append(env.Lines, Line{Level: LevelOK, Text: "Go runtime not required"})

	checking := opts.Root
	if abs, err := filepath.Abs(opts.Root); err == nil {
		checking = abs
	}
	b, err := security.NewBoundary(opts.Root)
	if err != nil {
		repo.Lines = append(repo.Lines, Line{Level: LevelFail, Text: "repository boundary cannot be established"})
		repo.Lines = append(repo.Lines, Line{Level: LevelFail, Text: err.Error()})
	} else {
		checking = b.Root()
		repo.Lines = append(repo.Lines, Line{Level: LevelOK, Text: "repository detected"})
		repo.Lines = append(repo.Lines, Line{Level: LevelOK, Text: "repository boundary valid"})
	}

	st, err := storage.Open(opts.Home)
	if err != nil {
		storeSec.Lines = append(storeSec.Lines, Line{Level: LevelFail, Text: "storage unavailable"})
		storeSec.Lines = append(storeSec.Lines, Line{Level: LevelFail, Text: err.Error()})
	} else {
		storeSec.Lines = append(storeSec.Lines, Line{Level: LevelOK, Text: "storage available"})
		if err := probeWritable(st); err != nil {
			storeSec.Lines = append(storeSec.Lines, Line{Level: LevelFail, Text: "storage unwritable"})
		} else {
			storeSec.Lines = append(storeSec.Lines, Line{Level: LevelOK, Text: "storage writable"})
		}
		if corrupt, err := sampleManifests(st); err != nil {
			storeSec.Lines = append(storeSec.Lines, Line{Level: LevelFail, Text: "checkpoint store unreadable"})
		} else if corrupt {
			storeSec.Lines = append(storeSec.Lines, Line{Level: LevelFail, Text: "checkpoint store corrupted"})
		} else {
			storeSec.Lines = append(storeSec.Lines, Line{Level: LevelOK, Text: "manifest format supported"})
		}
	}

	if b != nil && st != nil {
		probe, err := st.ProbeLock(b.Root())
		if err != nil {
			repo.Lines = append(repo.Lines, Line{Level: LevelFail, Text: "lock cannot be acquired/probed"})
		} else if probe.Held {
			extra := ""
			if probe.PID > 0 {
				extra = fmt.Sprintf("pid: %d", probe.PID)
			}
			repo.Lines = append(repo.Lines, Line{Level: LevelWarn, Text: "repository lock currently held", Extra: extra})
		} else {
			repo.Lines = append(repo.Lines, Line{Level: LevelOK, Text: "repository lock available"})
		}
	}

	userHome := opts.UserHome
	if userHome == "" {
		userHome, _ = os.UserHomeDir()
	}
	if b != nil && userHome != "" && security.SameDirectory(b.Root(), userHome) {
		repo.Lines = append(repo.Lines, Line{
			Level: LevelWarn,
			Text:  "current directory is your home directory",
			Extra: "agent-undo run will refuse to checkpoint $HOME",
		})
	}

	if b != nil {
		fsSec.Lines = append(fsSec.Lines, Line{Level: LevelOK, Text: "repository boundary enforced"})
		fsSec.Lines = append(fsSec.Lines, Line{Level: LevelOK, Text: "symlink policy available"})
		findings, walkFail, walkWarn := shallowWalk(b)
		if walkFail {
			fsSec.Lines = append(fsSec.Lines, Line{Level: LevelFail, Text: "workspace cannot be inspected safely"})
		} else {
			if walkWarn {
				fsSec.Lines = append(fsSec.Lines, Line{Level: LevelWarn, Text: "filesystem inspection incomplete or capped"})
			}
			for _, f := range findings {
				fsSec.Lines = append(fsSec.Lines, Line{Level: LevelWarn, Text: f})
			}
		}
	} else {
		fsSec.Lines = append(fsSec.Lines, Line{Level: LevelFail, Text: "repository boundary enforced: unavailable"})
	}

	gitSec.Lines = gitLines(ctx, opts.Root, b)

	support.Lines = []Line{
		{Level: LevelOK, Text: "local filesystem restore"},
		{Level: LevelNote, Text: "Git index/staging fidelity is not captured"},
		{Level: LevelNote, Text: "external side effects are not reversible"},
		{Level: LevelNote, Text: "Cursor/editor-native attachment is not supported in v0.1"},
	}

	rep := Report{Checking: checking, Sections: []Section{env, repo, storeSec, fsSec, gitSec, support}}
	rep.Status = statusOf(rep)
	return rep
}

func statusOf(r Report) Status {
	fail, warn := false, false
	for _, s := range r.Sections {
		for _, ln := range s.Lines {
			switch ln.Level {
			case LevelFail:
				fail = true
			case LevelWarn:
				warn = true
			}
		}
	}
	if fail {
		return StatusNotReady
	}
	if warn {
		return StatusReadyWithWarnings
	}
	return StatusReady
}

func supportedOS(goos string) (label string, ok bool) {
	switch goos {
	case "darwin":
		return "macOS", true
	case "linux":
		return "Linux", true
	case "windows":
		return "Windows", false
	default:
		return goos, false
	}
}

func probeWritable(st *storage.Store) error {
	f, err := os.CreateTemp(st.Home, ".doctor-probe-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := f.Write([]byte("ok\n")); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func sampleManifests(st *storage.Store) (corrupt bool, err error) {
	dir := filepath.Join(st.Home, "manifests")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return true, nil
		}
		var head struct {
			SchemaVersion int `json:"schemaVersion"`
		}
		if err := json.Unmarshal(raw, &head); err != nil || head.SchemaVersion != supportedSchema {
			return true, nil
		}
		n++
		if n >= MaxManifestSample {
			break
		}
	}
	return false, nil
}

func gitLines(ctx context.Context, root string, b *security.Boundary) []Line {
	if b == nil {
		return []Line{{Level: LevelFail, Text: "git identity unreadable (no boundary)"}}
	}
	st, err := git.ReadIdentity(root)
	if err != nil {
		return []Line{{Level: LevelWarn, Text: "git metadata unreadable"}}
	}
	if !st.Present {
		return []Line{{Level: LevelNote, Text: "git repository not present (filesystem-only restore)"}}
	}
	var out []Line
	if _, err := exec.LookPath("git"); err != nil {
		out = append(out, Line{Level: LevelWarn, Text: "git executable not on PATH"})
	} else {
		out = append(out, Line{Level: LevelOK, Text: "git detected"})
	}
	if st.Captured || st.Unborn {
		out = append(out, Line{Level: LevelOK, Text: "HEAD readable"})
		out = append(out, Line{Level: LevelOK, Text: "branch state readable"})
	} else {
		out = append(out, Line{Level: LevelWarn, Text: "HEAD unreadable"})
	}
	_ = ctx
	return out
}

func shallowWalk(b *security.Boundary) (findings []string, fail, warn bool) {
	classes := map[string]struct{}{}
	visited := 0
	err := filepath.WalkDir(b.Root(), func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == b.Root() {
				return walkErr
			}
			warn = true
			return nil
		}
		if path == b.Root() {
			return nil
		}
		visited++
		if visited > MaxWalkVisit {
			warn = true
			return fs.SkipAll
		}
		rel, err := b.RelPath(path)
		if err != nil {
			warn = true
			return nil
		}
		depth := strings.Count(rel, "/") + 1
		if d.IsDir() {
			dec := ignore.Dir(d.Name())
			if dec.SkipDir {
				if dec.Class != ignore.ClassGitDir {
					classes[dec.Class] = struct{}{}
				}
				return filepath.SkipDir
			}
			if depth >= MaxWalkDepth {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			warn = true
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				warn = true
				return nil
			}
			if b.TargetEscapes(rel, target) {
				classes[ignore.ClassExternal] = struct{}{}
			}
		}
		return nil
	})
	if err != nil && !isSkipAll(err) {
		return nil, true, false
	}
	findings = classPhrases(classes)
	return findings, false, warn
}

func isSkipAll(err error) bool {
	return err == fs.SkipAll
}

func classPhrases(classes map[string]struct{}) []string {
	if len(classes) == 0 {
		return nil
	}
	keys := make([]string, 0, len(classes))
	for k := range classes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, phraseForClass(k))
	}
	return out
}

func phraseForClass(class string) string {
	switch class {
	case ignore.ClassNodeModules, ignore.ClassVendor:
		return "dependency tree detected: " + class
	case ignore.ClassDist, ignore.ClassBuild, ignore.ClassTarget, ignore.ClassNext:
		return "build output detected: " + class
	case ignore.ClassPycache:
		return "bytecode cache detected: __pycache__"
	case ignore.ClassVenv:
		return "virtualenv detected: venv"
	case ignore.ClassExternal:
		return "external symlink target detected"
	default:
		return class + " detected"
	}
}

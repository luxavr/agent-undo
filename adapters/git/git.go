package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// State is a read-only snapshot of git identity and path classes.
type State struct {
	Present  bool
	Captured bool
	Head     string
	Branch   string
	Detached bool
	Unborn   bool
	Tracked  map[string]struct{}
	Ignored  map[string]struct{}
}

// Inspect reads git metadata. It does not write and does not run hooks
// (hooksPath is set to /dev/null for any git subprocess).
func Inspect(ctx context.Context, root string) (State, error) {
	st, err := ReadIdentity(root)
	if err != nil || !st.Present {
		return st, err
	}
	tracked, ignored, err := listFiles(ctx, root)
	if err != nil {
		// HEAD may still be captured without git(1).
		return st, nil
	}
	st.Tracked = tracked
	st.Ignored = ignored
	return st, nil
}

// ReadIdentity reads HEAD/branch from .git without listing the worktree.
func ReadIdentity(root string) (State, error) {
	gitDir, err := gitDirPath(root)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	st := State{
		Present: true,
		Tracked: map[string]struct{}{},
		Ignored: map[string]struct{}{},
	}
	head, branch, detached, unborn, err := readHEAD(gitDir)
	if err != nil {
		return st, fmt.Errorf("read HEAD: %w", err)
	}
	st.Head = head
	st.Branch = branch
	st.Detached = detached
	st.Unborn = unborn
	st.Captured = head != "" || unborn
	return st, nil
}

func gitDirPath(root string) (string, error) {
	p := filepath.Join(root, ".git")
	fi, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return p, nil
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			return "", err
		}
		return resolved, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(b))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("unrecognized .git file")
	}
	dir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	return filepath.Clean(dir), nil
}

func readHEAD(gitDir string) (head, branch string, detached, unborn bool, err error) {
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", "", false, false, err
	}
	line := strings.TrimSpace(string(b))
	if strings.HasPrefix(line, "ref: ") {
		ref := strings.TrimSpace(strings.TrimPrefix(line, "ref: "))
		branch = strings.TrimPrefix(ref, "refs/heads/")
		sha, err := resolveRef(gitDir, ref)
		if err != nil || sha == "" {
			return "", branch, false, true, nil
		}
		return sha, branch, false, false, nil
	}
	if !isHexSHA(line) {
		return "", "", false, false, fmt.Errorf("ambiguous HEAD %q", line)
	}
	return line, "", true, false, nil
}

func resolveRef(gitDir, ref string) (string, error) {
	p := filepath.Join(gitDir, filepath.FromSlash(ref))
	if b, err := os.ReadFile(p); err == nil {
		return strings.TrimSpace(string(b)), nil
	}
	return packedRef(gitDir, ref)
}

func packedRef(gitDir, ref string) (string, error) {
	f, err := os.Open(filepath.Join(gitDir, "packed-refs"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		sha, name, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if name == ref {
			return sha, nil
		}
	}
	return "", sc.Err()
}

func isHexSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' {
			continue
		}
		return false
	}
	return true
}

func listFiles(ctx context.Context, root string) (tracked, ignored map[string]struct{}, err error) {
	tracked = map[string]struct{}{}
	ignored = map[string]struct{}{}
	out, err := gitOut(ctx, root, "ls-files", "-z", "--cached")
	if err != nil {
		return nil, nil, err
	}
	for _, p := range splitNull(out) {
		tracked[p] = struct{}{}
	}
	out, err = gitOut(ctx, root, "ls-files", "-z", "--others", "--ignored", "--exclude-standard")
	if err != nil {
		return tracked, ignored, nil
	}
	for _, p := range splitNull(out) {
		ignored[p] = struct{}{}
	}
	return tracked, ignored, nil
}

func gitOut(ctx context.Context, root string, args ...string) ([]byte, error) {
	all := append([]string{"-c", "core.hooksPath=/dev/null", "-C", root}, args...)
	cmd := exec.CommandContext(ctx, "git", all...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func splitNull(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := bytes.Split(b, []byte{0})
	var out []string
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, string(p))
	}
	return out
}

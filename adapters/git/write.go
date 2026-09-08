package git

import (
	"context"
	"fmt"
	"strings"
)

// ObjectExists reports whether sha is in the object database.
func ObjectExists(ctx context.Context, root, sha string) (bool, error) {
	if sha == "" {
		return false, nil
	}
	_, err := gitOut(ctx, root, "cat-file", "-e", sha)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// BranchExists reports whether refs/heads/<branch> exists.
func BranchExists(ctx context.Context, root, branch string) (bool, error) {
	if branch == "" {
		return false, nil
	}
	_, err := gitOut(ctx, root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// IsAncestor reports whether ancestor is an ancestor of rev.
func IsAncestor(ctx context.Context, root, ancestor, rev string) (bool, error) {
	if ancestor == "" || rev == "" {
		return false, nil
	}
	_, err := gitOut(ctx, root, "merge-base", "--is-ancestor", ancestor, rev)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// RevList returns SHAs reachable from rev but not from ancestor.
func RevList(ctx context.Context, root, ancestor, rev string) ([]string, error) {
	out, err := gitOut(ctx, root, "rev-list", "--reverse", ancestor+".."+rev)
	if err != nil {
		return nil, err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}

// MoveHEAD checks out the captured branch (or detached SHA) and resets hard to sha.
// It never deletes branches, force-pushes, or prunes.
func MoveHEAD(ctx context.Context, root string, branch string, detached bool, sha string) error {
	if sha == "" {
		return fmt.Errorf("git: empty SHA")
	}
	ok, err := ObjectExists(ctx, root, sha)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("git: captured SHA %s is not in the object database", sha)
	}
	if detached || branch == "" {
		return gitRun(ctx, root, "checkout", "-f", "--detach", sha)
	}
	exists, err := BranchExists(ctx, root, branch)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("git: captured branch %q no longer exists", branch)
	}
	if err := gitRun(ctx, root, "checkout", "-f", branch); err != nil {
		return err
	}
	return gitRun(ctx, root, "reset", "--hard", sha)
}

func gitRun(ctx context.Context, root string, args ...string) error {
	_, err := gitOut(ctx, root, args...)
	return err
}

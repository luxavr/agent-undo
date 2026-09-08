package checkpoint

import (
	"errors"
	"fmt"
	"sort"

	"github.com/idlfirhan/agent-undo/internal/storage"
)

var (
	// ErrNoRecovery means this repository has no valid recovery checkpoint.
	ErrNoRecovery = errors.New("no recovery checkpoint for this repository.")
	// ErrNotRecovery means the id exists but is not kind=recovery.
	ErrNotRecovery = errors.New("not a recovery checkpoint")
	// ErrIncompleteRecovery means the recovery checkpoint cannot be loaded safely.
	ErrIncompleteRecovery = errors.New("checkpoint objects incomplete")
	// ErrRepoMismatch means the recovery checkpoint belongs to another boundary.
	ErrRepoMismatch = errors.New("recovery checkpoint repo root mismatch")
)

// LatestRecovery returns the newest valid recovery checkpoint for repoRoot.
// Ordering: createdAt descending, then id lexicographically descending.
// Incomplete manifests are skipped.
func LatestRecovery(store *storage.Store, repoRoot string) (*Manifest, error) {
	if store == nil || repoRoot == "" {
		return nil, ErrNoRecovery
	}
	ids, err := store.ListManifestIDs()
	if err != nil {
		return nil, err
	}
	var cand []*Manifest
	for _, id := range ids {
		m, err := Load(store, id)
		if err != nil {
			continue
		}
		if m.Kind != KindRecovery || m.RepoRoot != repoRoot {
			continue
		}
		if err := VerifyComplete(store, m); err != nil {
			continue
		}
		cand = append(cand, m)
	}
	if len(cand) == 0 {
		return nil, ErrNoRecovery
	}
	sort.Slice(cand, func(i, j int) bool {
		if !cand[i].CreatedAt.Equal(cand[j].CreatedAt) {
			return cand[i].CreatedAt.After(cand[j].CreatedAt)
		}
		return cand[i].ID > cand[j].ID
	})
	return cand[0], nil
}

// ResolveRecovery loads an explicit recovery checkpoint for repoRoot.
// Missing, incomplete, or wrong-repo targets fail closed.
func ResolveRecovery(store *storage.Store, id, repoRoot string) (*Manifest, error) {
	if store == nil || id == "" {
		return nil, fmt.Errorf("%w: missing id", ErrIncompleteRecovery)
	}
	m, err := Load(store, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIncompleteRecovery, err)
	}
	if m.Kind != KindRecovery {
		return nil, ErrNotRecovery
	}
	if m.RepoRoot != repoRoot {
		return nil, ErrRepoMismatch
	}
	if err := VerifyComplete(store, m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIncompleteRecovery, err)
	}
	return m, nil
}

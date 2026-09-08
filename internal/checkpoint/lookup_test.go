package checkpoint

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/luxavr/agent-undo/internal/security"
	"github.com/luxavr/agent-undo/internal/storage"
)

func TestLatestRecoveryOrdering(t *testing.T) {
	t.Parallel()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := "/repo/a"
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	putRecovery(t, store, "cp_oldoldoldold", root, older)
	putRecovery(t, store, "cp_newnewnewnew", root, newer)
	got, err := LatestRecovery(store, root)
	if err != nil || got.ID != "cp_newnewnewnew" {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestLatestRecoveryTieBreak(t *testing.T) {
	t.Parallel()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := "/repo/a"
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	putRecovery(t, store, "cp_aaaaaaaaaaaa", root, ts)
	putRecovery(t, store, "cp_zzzzzzzzzzzz", root, ts)
	got, err := LatestRecovery(store, root)
	if err != nil || got.ID != "cp_zzzzzzzzzzzz" {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestLatestRecoverySkipsOtherRepoAndIncomplete(t *testing.T) {
	t.Parallel()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := "/repo/a"
	ts := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	putRecovery(t, store, "cp_goodgoodgood", root, ts)
	putRecovery(t, store, "cp_otherotherot", "/repo/b", ts.Add(time.Hour))
	putIncompleteRecovery(t, store, "cp_badbadbadbad", root, ts.Add(2*time.Hour))
	putSession(t, store, "cp_sessionsess", root, ts.Add(3*time.Hour))
	got, err := LatestRecovery(store, root)
	if err != nil || got.ID != "cp_goodgoodgood" {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestLatestRecoveryNone(t *testing.T) {
	t.Parallel()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = LatestRecovery(store, "/repo/none")
	if err != ErrNoRecovery {
		t.Fatalf("%v", err)
	}
}

func TestResolveRecoveryRejectsSessionKind(t *testing.T) {
	t.Parallel()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := "/repo/a"
	putSession(t, store, "cp_sessionsess", root, time.Now().UTC())
	_, err = ResolveRecovery(store, "cp_sessionsess", root)
	if err != ErrNotRecovery {
		t.Fatalf("%v", err)
	}
}

func TestResolveRecoveryIncomplete(t *testing.T) {
	t.Parallel()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := "/repo/a"
	putIncompleteRecovery(t, store, "cp_incomplete01", root, time.Now().UTC())
	_, err = ResolveRecovery(store, "cp_incomplete01", root)
	if !errors.Is(err, ErrIncompleteRecovery) {
		t.Fatalf("%v", err)
	}
}

func TestResolveRecoveryMissing(t *testing.T) {
	t.Parallel()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = ResolveRecovery(store, "cp_doesnotexist1", "/repo/a")
	if !errors.Is(err, ErrIncompleteRecovery) {
		t.Fatalf("%v", err)
	}
}

func TestResolveRecoveryRepoMismatch(t *testing.T) {
	t.Parallel()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	putRecovery(t, store, "cp_mismatch0000", "/repo/a", time.Now().UTC())
	_, err = ResolveRecovery(store, "cp_mismatch0000", "/repo/b")
	if err != ErrRepoMismatch {
		t.Fatalf("%v", err)
	}
}

func TestOldRecoveryWithoutSourceLoads(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Create(context.Background(), Options{Boundary: b, Store: store, Kind: KindRecovery})
	if err != nil {
		t.Fatal(err)
	}
	if m.Source.Type != "" {
		t.Fatalf("%+v", m.Source)
	}
	got, err := LatestRecovery(store, b.Root())
	if err != nil || got.ID != m.ID {
		t.Fatalf("%v %+v", err, got)
	}
}

func putRecovery(t *testing.T, store *storage.Store, id, repoRoot string, created time.Time) {
	t.Helper()
	writeManifest(t, store, Manifest{
		SchemaVersion: schemaVersion,
		ID:            id,
		Kind:          KindRecovery,
		RepoRoot:      repoRoot,
		CreatedAt:     created,
	})
}

func putSession(t *testing.T, store *storage.Store, id, repoRoot string, created time.Time) {
	t.Helper()
	writeManifest(t, store, Manifest{
		SchemaVersion: schemaVersion,
		ID:            id,
		Kind:          KindSession,
		RepoRoot:      repoRoot,
		CreatedAt:     created,
	})
}

func putIncompleteRecovery(t *testing.T, store *storage.Store, id, repoRoot string, created time.Time) {
	t.Helper()
	writeManifest(t, store, Manifest{
		SchemaVersion: schemaVersion,
		ID:            id,
		Kind:          KindRecovery,
		RepoRoot:      repoRoot,
		CreatedAt:     created,
		Files:         []File{{Path: "a", Kind: KindFile, SHA256: "aa", Size: 1}},
	})
}

func writeManifest(t *testing.T, store *storage.Store, m Manifest) {
	t.Helper()
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := store.WriteManifest(m.ID, raw); err != nil {
		t.Fatal(err)
	}
}

package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store is the local object/manifest home.
type Store struct {
	Home string
}

// DefaultHome is AGENT_UNDO_HOME or ~/.agent-undo.
func DefaultHome() (string, error) {
	if v := os.Getenv("AGENT_UNDO_HOME"); v != "" {
		return v, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".agent-undo"), nil
}

// Open creates the store layout.
func Open(home string) (*Store, error) {
	if home == "" {
		return nil, fmt.Errorf("empty store home")
	}
	s := &Store{Home: home}
	for _, d := range []string{
		s.objectsDir(),
		filepath.Join(home, "manifests"),
		filepath.Join(home, "sessions"),
		filepath.Join(home, "locks"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) objectsDir() string { return filepath.Join(s.Home, "objects") }

func (s *Store) objectPath(sum string) string {
	if len(sum) < 4 {
		return filepath.Join(s.objectsDir(), sum)
	}
	return filepath.Join(s.objectsDir(), sum[:2], sum[2:4], sum)
}

// PutBytes writes a blob if missing and returns the hex SHA-256.
func (s *Store) PutBytes(b []byte) (string, error) {
	sum := sha256.Sum256(b)
	hexSum := hex.EncodeToString(sum[:])
	dest := s.objectPath(hexSum)
	if _, err := os.Stat(dest); err == nil {
		return hexSum, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".tmp-")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", err
	}
	ok = true
	return hexSum, nil
}

// PutFrom streams r into a new object and returns hex SHA-256 and byte count.
func (s *Store) PutFrom(r io.Reader) (string, int64, error) {
	if err := os.MkdirAll(s.objectsDir(), 0o755); err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp(s.objectsDir(), ".tmp-")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		_ = tmp.Close()
		return "", 0, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	hexSum := hex.EncodeToString(h.Sum(nil))
	dest := s.objectPath(hexSum)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", 0, err
	}
	if _, err := os.Stat(dest); err == nil {
		_ = os.Remove(tmpName)
		ok = true
		return hexSum, n, nil
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", 0, err
	}
	ok = true
	return hexSum, n, nil
}

// GetBytes reads a blob and verifies its hash.
func (s *Store) GetBytes(sum string) ([]byte, error) {
	b, err := os.ReadFile(s.objectPath(sum))
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(b)
	if hex.EncodeToString(got[:]) != sum {
		return nil, fmt.Errorf("object %s failed hash verify", sum)
	}
	return b, nil
}

// Has reports whether an object exists (without hashing).
func (s *Store) Has(sum string) bool {
	_, err := os.Stat(s.objectPath(sum))
	return err == nil
}

// WriteManifest writes manifests/<id>.json atomically.
func (s *Store) WriteManifest(id string, data []byte) error {
	if id == "" {
		return fmt.Errorf("empty manifest id")
	}
	dir := filepath.Join(s.Home, "manifests")
	dest := filepath.Join(dir, id+".json")
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
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
	return os.Rename(tmpName, dest)
}

// ReadManifest reads manifests/<id>.json.
func (s *Store) ReadManifest(id string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.Home, "manifests", id+".json"))
}

// ListManifestIDs returns manifest ids (filename minus .json), sorted.
func (s *Store) ListManifestIDs() ([]string, error) {
	dir := filepath.Join(s.Home, "manifests")
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range ents {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(ids)
	return ids, nil
}

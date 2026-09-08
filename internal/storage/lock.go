package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ErrLocked is returned when the exclusive repository lock is held.
var ErrLocked = errors.New("repository locked")

// Lock is an exclusive repository lock. Unlock does not steal; it only
// releases a lock this process holds.
type Lock struct {
	f *os.File
}

func (s *Store) lockPath(repoRoot string) string {
	sum := sha256.Sum256([]byte(repoRoot))
	return filepath.Join(s.Home, "locks", hex.EncodeToString(sum[:])+".lock")
}

// TryLock acquires an exclusive non-blocking lock for repoRoot.
func (s *Store) TryLock(repoRoot string) (*Lock, error) {
	if repoRoot == "" {
		return nil, fmt.Errorf("empty repo root")
	}
	path := s.lockPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%w (doctor can diagnose held locks)", ErrLocked)
	}
	if err := f.Truncate(0); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, err
	}
	if _, err := f.WriteString(fmt.Sprintf("%d\n", os.Getpid())); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, err
	}
	_ = f.Sync()
	return &Lock{f: f}, nil
}

// LockProbe is the result of a doctor lock probe. It does not steal a held lock.
type LockProbe struct {
	Held bool
	PID  int
}

func lockHeldErrno(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EACCES)
}

func readPID(r io.Reader) int {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return n
}

// ProbeLock reports whether flock is free. If free, it acquires, writes a PID
// marker, and immediately releases. It never deletes the lock file and never
// steals a held lock.
func (s *Store) ProbeLock(repoRoot string) (LockProbe, error) {
	if repoRoot == "" {
		return LockProbe{}, fmt.Errorf("empty repo root")
	}
	path := s.lockPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return LockProbe{}, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return LockProbe{}, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_, _ = f.Seek(0, io.SeekStart)
		pid := readPID(f)
		_ = f.Close()
		if lockHeldErrno(err) {
			return LockProbe{Held: true, PID: pid}, nil
		}
		return LockProbe{}, err
	}
	if err := f.Truncate(0); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return LockProbe{}, err
	}
	if _, err := f.WriteString(fmt.Sprintf("%d\n", os.Getpid())); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return LockProbe{}, err
	}
	_ = f.Sync()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		_ = f.Close()
		return LockProbe{}, err
	}
	if err := f.Close(); err != nil {
		return LockProbe{}, err
	}
	return LockProbe{Held: false}, nil
}

// Unlock releases the lock.
func (l *Lock) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}

package storage

import (
	"os"
	"testing"
)

func TestProbeLockAvailable(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.ProbeLock("/repo/a")
	if err != nil || p.Held {
		t.Fatalf("%+v %v", p, err)
	}
	p2, err := s.ProbeLock("/repo/a")
	if err != nil || p2.Held {
		t.Fatal("leftover lock file must not be treated as held")
	}
	l, err := s.TryLock("/repo/a")
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Unlock()
}

func TestProbeLockHeld(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l, err := s.TryLock("/repo/a")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Unlock()
	p, err := s.ProbeLock("/repo/a")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Held {
		t.Fatal("expected held")
	}
	if p.PID != os.Getpid() {
		t.Fatalf("pid %d want %d", p.PID, os.Getpid())
	}
	if _, err := s.TryLock("/repo/a"); err == nil {
		t.Fatal("probe must not steal")
	}
}

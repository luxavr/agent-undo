package storage

import "testing"

func TestTryLockExclusive(t *testing.T) {
	t.Parallel()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l1, err := s.TryLock("/repo/a")
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Unlock()
	if _, err := s.TryLock("/repo/a"); err == nil {
		t.Fatal("second lock must fail closed")
	}
	if err := l1.Unlock(); err != nil {
		t.Fatal(err)
	}
	l2, err := s.TryLock("/repo/a")
	if err != nil {
		t.Fatal(err)
	}
	_ = l2.Unlock()
}

func TestLockDifferentRepos(t *testing.T) {
	t.Parallel()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.TryLock("/repo/a")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Unlock()
	b, err := s.TryLock("/repo/b")
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Unlock()
}

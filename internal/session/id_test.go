package session

import (
	"strings"
	"testing"
)

func TestNewID(t *testing.T) {
	t.Parallel()
	a := NewID("cp")
	b := NewID("cp")
	if a == b || len(a) < 8 || a[:3] != "cp_" {
		t.Fatalf("%q %q", a, b)
	}
}

func TestNewSessionID(t *testing.T) {
	t.Parallel()
	a := NewSessionID()
	b := NewSessionID()
	if a == b {
		t.Fatal("collision")
	}
	if !ValidSessionID(a) {
		t.Fatalf("%q", a)
	}
	if strings.ToUpper(a) != a {
		t.Fatalf("must persist uppercase: %q", a)
	}
}

func TestCanonicalSessionID(t *testing.T) {
	t.Parallel()
	id := NewSessionID()
	got, err := CanonicalSessionID(strings.ToLower(id))
	if err != nil || got != id {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := CanonicalSessionID("cp_abc"); err == nil {
		t.Fatal("cp id must not be a session id")
	}
	if !LooksLikeCheckpointID("cp_deadbeef") {
		t.Fatal()
	}
}

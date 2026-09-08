package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestPutGetHas(t *testing.T) {
	t.Parallel()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sum, err := s.PutBytes([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte("hello"))
	if sum != hex.EncodeToString(want[:]) {
		t.Fatalf("sum %s", sum)
	}
	if !s.Has(sum) {
		t.Fatal("missing")
	}
	got, err := s.GetBytes(sum)
	if err != nil || string(got) != "hello" {
		t.Fatalf("%q %v", got, err)
	}
	again, err := s.PutBytes([]byte("hello"))
	if err != nil || again != sum {
		t.Fatal(err)
	}
}

func TestCorruptObject(t *testing.T) {
	t.Parallel()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sum, err := s.PutBytes([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.objectPath(sum), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBytes(sum); err == nil {
		t.Fatal("expected hash failure")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	t.Parallel()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteManifest("abc", []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	b, err := s.ReadManifest("abc")
	if err != nil || string(b) != `{"ok":true}` {
		t.Fatalf("%s %v", b, err)
	}
}

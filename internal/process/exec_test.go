package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecuteTrue(t *testing.T) {
	t.Parallel()
	r := Execute(context.Background(), Options{Argv: []string{"true"}})
	if r.StartErr != nil || r.ExitCode != 0 || r.Interrupted {
		t.Fatalf("%+v", r)
	}
}

func TestExecuteFalse(t *testing.T) {
	t.Parallel()
	r := Execute(context.Background(), Options{Argv: []string{"false"}})
	if r.StartErr != nil || r.ExitCode != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestExecuteMissing(t *testing.T) {
	t.Parallel()
	r := Execute(context.Background(), Options{Argv: []string{"agent-undo-no-such-bin"}})
	if r.StartErr == nil {
		t.Fatal("expected start error")
	}
}

func TestExecuteLiteralArg(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	script := filepath.Join(dir, "echoarg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\" > \"$2\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	r := Execute(context.Background(), Options{
		Argv: []string{script, "hello world; rm -rf /", out},
		Dir:  dir,
	})
	if r.StartErr != nil || r.ExitCode != 0 {
		t.Fatalf("%+v", r)
	}
	b, err := os.ReadFile(out)
	if err != nil || string(b) != "hello world; rm -rf /" {
		t.Fatalf("%q %v", b, err)
	}
}

func TestExecuteCancelEscalation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	r := Execute(ctx, Options{Argv: []string{"sleep", "30"}, Grace: 30 * time.Millisecond})
	if r.StartErr != nil {
		t.Fatal(r.StartErr)
	}
	if !r.Interrupted {
		t.Fatalf("want interrupted %+v", r)
	}
}

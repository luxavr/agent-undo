package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luxavr/agent-undo/internal/security"
)

func TestCLIDoctorReady(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("AGENT_UNDO_HOME", home)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	stdout, stderr, code := run([]string{"doctor"})
	if code != 0 {
		t.Fatalf("code %d stderr %q stdout %q", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "AGENT UNDO DOCTOR") || !strings.Contains(stdout, "Status:\n  READY") {
		t.Fatalf("%q", stdout)
	}
	if strings.Contains(stdout, "SUCCESS") || strings.Contains(stdout, "FAILED") {
		t.Fatal("doctor must not use verify vocabulary")
	}
	b, err := security.NewBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "checking: "+b.Root()) {
		t.Fatalf("doctor must print the directory it inspects:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Agent Undo CLI available") {
		t.Fatalf("doctor must report CLI availability, not repo setup:\n%s", stdout)
	}
	if strings.Contains(stdout, "Agent Undo installed") {
		t.Fatal("doctor must not say installed")
	}
}

func TestCLIDoctorWarnings(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_UNDO_HOME", t.TempDir())
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	stdout, _, code := run([]string{"doctor"})
	if code != 0 {
		t.Fatalf("warnings must exit 0, got %d %s", code, stdout)
	}
	if !strings.Contains(stdout, "READY WITH WARNINGS") {
		t.Fatal(stdout)
	}
}

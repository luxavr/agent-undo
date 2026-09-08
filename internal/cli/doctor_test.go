package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

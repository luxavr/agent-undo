package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luxavr/agent-undo/internal/version"
)

func TestHelp(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {}, {"--help"}, {"-h"}, {"help"}} {
		stdout, stderr, code := run(args)
		if code != 0 {
			t.Fatalf("args %v: code %d stderr %q", args, code, stderr)
		}
		if !strings.Contains(stdout, "session show") || !strings.Contains(stdout, "run <agent command>") {
			t.Fatalf("help missing canonical commands:\n%s", stdout)
		}
		if strings.Contains(stdout, "inspect") {
			t.Fatalf("help must not advertise inspect:\n%s", stdout)
		}
		if !strings.Contains(stdout, "Ctrl+Z for a wrapped terminal AI coding session") {
			t.Fatalf("help hero must match wrapper-only identity:\n%s", stdout)
		}
		if !strings.Contains(stdout, "Cursor/editor-native attachment is not supported") {
			t.Fatalf("help must state wrapper-only v0.1:\n%s", stdout)
		}
		if !strings.Contains(stdout, "Does not initialize the repository") {
			t.Fatalf("help must say doctor does not initialize:\n%s", stdout)
		}
		if strings.Contains(stdout, "not supported yet") {
			t.Fatal("help must not imply a scheduled Cursor attach")
		}
		if !strings.Contains(stdout, "--yes requires an explicit cp_ id") {
			t.Fatalf("help must state recover --yes needs a target:\n%s", stdout)
		}
		if !strings.Contains(stdout, "run refuses to checkpoint the user home directory") {
			t.Fatalf("help must state home refusal:\n%s", stdout)
		}
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		stdout, stderr, code := run(args)
		if code != 0 || stderr != "" {
			t.Fatalf("args %v: code %d stderr %q", args, code, stderr)
		}
		if strings.TrimSpace(stdout) != version.Version {
			t.Fatalf("got %q want %q", stdout, version.Version)
		}
	}
}

func TestSessionListEmpty(t *testing.T) {
	t.Setenv("AGENT_UNDO_HOME", t.TempDir())
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	stdout, stderr, code := run([]string{"session", "list"})
	if code != 0 || stderr != "" {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "no sessions") {
		t.Fatalf("%q", stdout)
	}
}

func TestUsageErrors(t *testing.T) {
	t.Parallel()
	cases := [][]string{
		{"nope"},
		{"run"},
		{"run", "--"},
		{"session"},
		{"session", "inspect"},
		{"session", "show"},
		{"undo"},
		{"verify"},
		{"diff"},
		{"undo", "--yes"},
		{"undo", "id", "extra"},
		{"doctor", "extra"},
		{"recover", "--bogus"},
		{"recover", "cp_a", "cp_b"},
	}
	for _, args := range cases {
		_, stderr, code := run(args)
		if code != exitUsage {
			t.Fatalf("%v: code %d stderr %q", args, code, stderr)
		}
	}
}

func run(args []string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func TestCLIRunRefusesHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AGENT_UNDO_HOME", t.TempDir())
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	_, stderr, code := run([]string{"run", "true"})
	if code != exitUsage || !strings.Contains(stderr, "refusing to checkpoint the home directory") {
		t.Fatalf("%d %q", code, stderr)
	}
}

func TestCLIRunAllowsProjectUnderHome(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, "Projects", "foo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("AGENT_UNDO_HOME", t.TempDir())
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	_, stderr, code := run([]string{"run", "true"})
	if code != 0 {
		t.Fatalf("nested project must be allowed: %d %q", code, stderr)
	}
}

func TestCLIDoctorHomeWarning(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
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
		t.Fatalf("%d %s", code, stdout)
	}
	if !strings.Contains(stdout, "READY WITH WARNINGS") {
		t.Fatalf("%s", stdout)
	}
	if !strings.Contains(stdout, "current directory is your home directory") {
		t.Fatalf("%s", stdout)
	}
	if !strings.Contains(stdout, "agent-undo run will refuse to checkpoint $HOME") {
		t.Fatalf("%s", stdout)
	}
}

package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
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
		if !strings.Contains(stdout, "Cursor/editor-native attachment is not supported") {
			t.Fatalf("help must state wrapper-only v0.1:\n%s", stdout)
		}
		if strings.Contains(stdout, "not supported yet") {
			t.Fatal("help must not imply a scheduled Cursor attach")
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
		if strings.TrimSpace(stdout) != Version {
			t.Fatalf("got %q", stdout)
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

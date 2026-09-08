package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIUndoVerifyDiffSession(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("AGENT_UNDO_HOME", home)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "agent.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf new > a.txt\nprintf extra > b.txt\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"run", script}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run: code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "AGENT UNDO") || !strings.Contains(out, "Undo:") || strings.Contains(out, "[keep]") {
		t.Fatalf("receipt:\n%s", out)
	}
	id := sessionIDFromShow(t, out)
	receipt := out[strings.Index(out, "AGENT UNDO"):]
	if !strings.Contains(receipt, "Checkpoint:\n  VERIFIED") || !strings.Contains(receipt, "Undo:\n  AVAILABLE") {
		t.Fatalf("run receipt:\n%s", receipt)
	}
	if strings.Contains(receipt, "SUCCESS") || strings.Contains(receipt, "FAILED") {
		t.Fatalf("run receipt used verify vocabulary:\n%s", receipt)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"session", "list"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), id) || !strings.Contains(stdout.String(), "SESSION ID") {
		t.Fatalf("list: %d %q %q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"session", "show", id}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "AGENT UNDO") || !strings.Contains(stdout.String(), "Undo:") {
		t.Fatalf("show: %d %q %q", code, stdout.String(), stderr.String())
	}
	if stdout.String() != receipt {
		t.Fatalf("session show must match run receipt\nshow:\n%s\nrun:\n%s", stdout.String(), receipt)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"diff", id}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "added:") {
		t.Fatalf("diff: %d %q %q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"verify", "cp_deadbeefdead"}, &stdout, &stderr)
	if code != exitUsage || !strings.Contains(stderr.String(), "session id") {
		t.Fatalf("verify cp: %d %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"undo", "--yes", "cp_deadbeefdead"}, &stdout, &stderr)
	if code != exitUsage || !strings.Contains(stderr.String(), "session id") {
		t.Fatalf("undo cp: %d %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"verify", id}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("verify should fail on mutation: %s %s", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"undo", "--yes", id}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "SUCCESS") {
		t.Fatalf("undo: code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	body, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil || string(body) != "old" {
		t.Fatalf("got %q %v", body, err)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"verify", id}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "SUCCESS") {
		t.Fatalf("verify after undo: %d %q %q", code, stdout.String(), stderr.String())
	}
}

func TestCLIRunNestedLockMessage(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() {
		var stdout, stderr bytes.Buffer
		done <- Run(ctx, []string{"run", "sleep", "30"}, &stdout, &stderr)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("first run never reached RUNNING")
		}
		var stdout, stderr bytes.Buffer
		_ = Run(context.Background(), []string{"session", "list"}, &stdout, &stderr)
		if strings.Contains(stdout.String(), "RUNNING") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	var stdout, stderr bytes.Buffer
	secondCode := Run(context.Background(), []string{"run", "true"}, &stdout, &stderr)
	secondErr := stderr.String()
	if secondCode != exitInternal {
		t.Fatalf("second run: code %d stderr %q stdout %q", secondCode, secondErr, stdout.String())
	}
	if strings.TrimSpace(strings.Split(secondErr, "\n")[0]) != "Agent Undo session already active for this repository." {
		t.Fatalf("stable message missing:\n%s", secondErr)
	}
	cancel()
	<-done
}

func sessionIDFromShow(t *testing.T, out string) string {
	t.Helper()
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "Session:" && i+1 < len(lines) {
			id := strings.TrimSpace(lines[i+1])
			if id == "" || id == "-" {
				t.Fatalf("empty session id in %q", out)
			}
			return id
		}
	}
	t.Fatalf("no Session: line in %q", out)
	return ""
}

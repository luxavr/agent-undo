package doctor

import (
	"strings"
	"testing"
)

func TestRenderReady(t *testing.T) {
	t.Parallel()
	got := Render(Report{
		Status: StatusReady,
		Sections: []Section{
			{Title: "Environment", Lines: []Line{{Level: LevelOK, Text: "Agent Undo installed"}}},
			{Title: "Support", Lines: []Line{
				{Level: LevelOK, Text: "local filesystem restore"},
				{Level: LevelNote, Text: "Git index/staging fidelity is not captured"},
			}},
		},
	})
	want := `AGENT UNDO DOCTOR

Environment
  ✓ Agent Undo installed

Support
  ✓ local filesystem restore
  - Git index/staging fidelity is not captured

Status:
  READY
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderChecking(t *testing.T) {
	t.Parallel()
	got := Render(Report{Checking: "/tmp/repo", Status: StatusReady})
	if !strings.HasPrefix(got, "AGENT UNDO DOCTOR\n\nchecking: /tmp/repo\n") {
		t.Fatal(got)
	}
}

func TestRenderHeldLock(t *testing.T) {
	t.Parallel()
	got := Render(Report{
		Status: StatusReadyWithWarnings,
		Sections: []Section{{
			Title: "Repository",
			Lines: []Line{{Level: LevelWarn, Text: "repository lock currently held", Extra: "pid: 12345"}},
		}},
	})
	if !strings.Contains(got, "⚠ repository lock currently held") || !strings.Contains(got, "pid: 12345") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "READY WITH WARNINGS") {
		t.Fatal(got)
	}
}

func TestStatusIgnoresNotes(t *testing.T) {
	t.Parallel()
	r := Report{Sections: []Section{{
		Lines: []Line{
			{Level: LevelOK, Text: "ok"},
			{Level: LevelNote, Text: "external side effects are not reversible"},
		},
	}}}
	if statusOf(r) != StatusReady {
		t.Fatalf("%s", statusOf(r))
	}
}

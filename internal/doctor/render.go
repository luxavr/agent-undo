package doctor

import (
	"fmt"
	"strings"
)

const (
	markOK   = "✓"
	markWarn = "⚠"
	markFail = "✗"
	markNote = "-"
)

// Render formats a doctor report. Pure: no IO, clock, or environment.
func Render(r Report) string {
	var b strings.Builder
	b.WriteString("AGENT UNDO DOCTOR\n")
	if r.Checking != "" {
		fmt.Fprintf(&b, "\nchecking: %s\n", r.Checking)
	}
	for _, s := range r.Sections {
		b.WriteByte('\n')
		b.WriteString(s.Title)
		b.WriteByte('\n')
		for _, ln := range s.Lines {
			fmt.Fprintf(&b, "  %s %s\n", mark(ln.Level), ln.Text)
			if ln.Extra != "" {
				fmt.Fprintf(&b, "    %s\n", ln.Extra)
			}
		}
	}
	fmt.Fprintf(&b, "\nStatus:\n  %s\n", r.Status)
	return b.String()
}

func mark(l Level) string {
	switch l {
	case LevelWarn:
		return markWarn
	case LevelFail:
		return markFail
	case LevelNote:
		return markNote
	default:
		return markOK
	}
}

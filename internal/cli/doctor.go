package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/idlfirhan/agent-undo/internal/doctor"
	"github.com/idlfirhan/agent-undo/internal/storage"
)

func cmdDoctor(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintf(stderr, "usage: agent-undo doctor\n")
		return exitUsage
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "doctor: %v\n", err)
		return exitInternal
	}
	home, err := storage.DefaultHome()
	if err != nil {
		fmt.Fprintf(stderr, "doctor: %v\n", err)
		return exitInternal
	}
	rep := doctor.Inspect(ctx, doctor.Options{Root: wd, Home: home})
	fmt.Fprint(stdout, doctor.Render(rep))
	if rep.Status == doctor.StatusNotReady {
		return exitInternal
	}
	return exitOK
}

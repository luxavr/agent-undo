package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/luxavr/agent-undo/internal/doctor"
	"github.com/luxavr/agent-undo/internal/storage"
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
	userHome, err := os.UserHomeDir()
	if err != nil {
		userHome = ""
	}
	rep := doctor.Inspect(ctx, doctor.Options{Root: wd, Home: home, UserHome: userHome})
	fmt.Fprint(stdout, doctor.Render(rep))
	if rep.Status == doctor.StatusNotReady {
		return exitInternal
	}
	return exitOK
}

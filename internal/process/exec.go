package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	defaultGrace = 2 * time.Second
	MarkerEnv    = "AGENT_UNDO_SESSION"
)

// Result is the child process outcome. It is not the Agent Undo outcome.
type Result struct {
	ExitCode    int
	Signal      string
	Interrupted bool
	StartErr    error
}

// Options for Execute.
type Options struct {
	Argv     []string
	Dir      string
	Stdout   io.Writer
	Stderr   io.Writer
	ExtraEnv []string
	Grace    time.Duration
}

// Execute runs argv directly (no shell). The child is placed in its own
// process group. On cancel: SIGINT → grace → SIGTERM → grace → SIGKILL.
func Execute(ctx context.Context, opts Options) Result {
	if len(opts.Argv) == 0 {
		return Result{StartErr: fmt.Errorf("empty argv")}
	}
	grace := opts.Grace
	if grace <= 0 {
		grace = defaultGrace
	}
	path, err := exec.LookPath(opts.Argv[0])
	if err != nil {
		return Result{StartErr: err}
	}
	cmd := exec.Command(path, opts.Argv[1:]...)
	cmd.Dir = opts.Dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Env = append(os.Environ(), opts.ExtraEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return Result{StartErr: err}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return waitResult(err, false)
	case <-ctx.Done():
		killGroup(cmd, syscall.SIGINT)
		if err, ok := recv(done, grace); ok {
			return waitResult(err, true)
		}
		killGroup(cmd, syscall.SIGTERM)
		if err, ok := recv(done, grace); ok {
			return waitResult(err, true)
		}
		killGroup(cmd, syscall.SIGKILL)
		return waitResult(<-done, true)
	}
}

func recv(done <-chan error, d time.Duration) (error, bool) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case err := <-done:
		return err, true
	case <-t.C:
		return nil, false
	}
}

func killGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}

func waitResult(err error, interrupted bool) Result {
	r := Result{Interrupted: interrupted}
	if err == nil {
		return r
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		ws, ok := ee.Sys().(syscall.WaitStatus)
		if ok {
			if ws.Signaled() {
				r.Signal = ws.Signal().String()
				r.ExitCode = 128 + int(ws.Signal())
			} else {
				r.ExitCode = ws.ExitStatus()
			}
		} else {
			r.ExitCode = ee.ExitCode()
		}
		return r
	}
	r.StartErr = err
	return r
}

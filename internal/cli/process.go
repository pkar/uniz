package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// lookPath finds name like execvp, searching pathList (the PATH the child
// will see) when name has no separator.
func lookPath(name, pathList string) (string, error) {
	if strings.ContainsAny(name, `/`+string(filepath.Separator)) {
		return exec.LookPath(name)
	}
	for _, dir := range filepath.SplitList(pathList) {
		if dir == "" {
			dir = "."
		}
		if p, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s: %w", name, exec.ErrNotFound)
}

func getenv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && k == key {
			return v
		}
	}
	return ""
}

// process describes an external program run by env, xargs, or timeout.
type process struct {
	argv    []string
	env     []string  // nil inherits the process environment
	stdin   io.Reader // nil means the null device
	cancel  func(*exec.Cmd) error
	delay   func(*exec.Cmd) // called before Start: adjust the command
	started func(*exec.Cmd) // called after a successful Start
	done    func(*exec.Cmd) // called after Wait returns
}

// exitStatus converts a child's result to a uniz error: nil, *ExitError with
// the child's status (or 128+signal), 127 when not found, 126 when it cannot
// be started.
func (p process) run(ctx context.Context, s stdio, command string) error {
	env := p.env
	if env == nil {
		env = os.Environ()
	}
	path, err := lookPath(p.argv[0], getenv(env, "PATH"))
	if err != nil {
		return &ExitError{Code: 127, Err: fmt.Errorf("%s: %w", command, err)}
	}
	cmd := exec.CommandContext(ctx, path, p.argv[1:]...)
	cmd.Args[0] = p.argv[0]
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = s.out, s.err
	if p.cancel != nil {
		cmd.Cancel = func() error { return p.cancel(cmd) }
	}
	if p.delay != nil {
		p.delay(cmd)
	}
	var pipeWriter *os.File
	switch in := p.stdin.(type) {
	case nil:
	case *os.File:
		cmd.Stdin = in
	default:
		// Copy through our own pipe so a reader that blocks cannot hold up
		// Wait after the child exits; the copier is abandoned then.
		r, w, err := os.Pipe()
		if err != nil {
			return fmt.Errorf("%s: %w", command, err)
		}
		cmd.Stdin, pipeWriter = r, w
		defer r.Close()
		go func() {
			io.Copy(w, in)
			w.Close()
		}()
	}
	if err := cmd.Start(); err != nil {
		if pipeWriter != nil {
			pipeWriter.Close()
		}
		code := 126
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			code = 127
		}
		return &ExitError{Code: code, Err: fmt.Errorf("%s: %w", command, err)}
	}
	if p.started != nil {
		p.started(cmd)
	}
	err = cmd.Wait()
	if p.done != nil {
		p.done(cmd)
	}
	if pipeWriter != nil {
		pipeWriter.Close()
	}
	var exitErr *exec.ExitError
	switch {
	case err != nil && ctx.Err() != nil:
		return ctx.Err()
	case err == nil:
		return nil
	case errors.As(err, &exitErr):
		code := exitErr.ExitCode()
		if status, ok := exitErr.Sys().(interface {
			Signaled() bool
			Signal() syscall.Signal
		}); ok && status.Signaled() {
			code = 128 + int(status.Signal())
		}
		return &ExitError{Code: code}
	}
	return fmt.Errorf("%s: %w", command, err)
}

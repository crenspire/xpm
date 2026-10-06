package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// Command is one process to run in a project directory.
type Command struct {
	Dir  string   // working directory, set as cmd.Dir (xpm never changes its own)
	Name string   // executable
	Args []string // arguments
	Env  []string // extra KEY=VALUE pairs on top of the current environment
}

// String renders the command line for headers and error messages.
func (c Command) String() string {
	return strings.TrimSpace(strings.Join(append(append([]string{}, c.Env...), append([]string{c.Name}, c.Args...)...), " "))
}

// Runner runs c with its output sent to stdout and stderr. Tests inject a
// fake; nil means ExecRunner.
type Runner func(ctx context.Context, c Command, stdout, stderr io.Writer) error

// ExecRunner runs c with exec.CommandContext in c.Dir.
func ExecRunner(ctx context.Context, c Command, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	return cmd.Run()
}

// step is one labelled command of a workspace operation.
type step struct {
	label string // project name (or the workspace root's name)
	cmd   Command
	err   error // set instead of cmd when the step cannot run (e.g. tool missing)
}

// execOptions are the shared knobs of Install and Run.
type execOptions struct {
	parallel       bool
	run            Runner
	stdout, stderr io.Writer
}

// execute runs steps and returns their failures joined, in step order.
// Sequential runs stream each step's output after a "[label] $ cmd" header.
// Parallel runs (at most GOMAXPROCS at a time) buffer each step's output and
// print the header plus that output in one piece when the step finishes, so
// projects never interleave.
func execute(ctx context.Context, steps []step, o execOptions) error {
	if o.run == nil {
		o.run = ExecRunner
	}
	if o.stdout == nil {
		o.stdout = os.Stdout
	}
	if o.stderr == nil {
		o.stderr = os.Stderr
	}
	errs := make([]error, len(steps))
	runStep := func(i int, stdout, stderr io.Writer) {
		s := steps[i]
		if s.err != nil {
			errs[i] = fmt.Errorf("[%s] %w", s.label, s.err)
			return
		}
		if err := o.run(ctx, s.cmd, stdout, stderr); err != nil {
			errs[i] = fmt.Errorf("[%s] %s: %w", s.label, s.cmd, err)
		}
	}
	header := func(s step) string { return fmt.Sprintf("[%s] $ %s\n", s.label, s.cmd) }

	if !o.parallel {
		for i, s := range steps {
			if s.err == nil {
				_, _ = io.WriteString(o.stdout, header(s))
			}
			runStep(i, o.stdout, o.stderr)
		}
		return errors.Join(errs...)
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, runtime.GOMAXPROCS(0))
	)
	for i := range steps {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var out, errOut bytes.Buffer
			runStep(i, &out, &errOut)
			if steps[i].err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			_, _ = io.WriteString(o.stdout, header(steps[i]))
			_, _ = o.stdout.Write(out.Bytes())
			_, _ = o.stderr.Write(errOut.Bytes())
		}(i)
	}
	wg.Wait()
	return errors.Join(errs...)
}

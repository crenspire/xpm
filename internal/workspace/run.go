package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/crenspire/xpm/internal/scripts"
)

// RunOptions configures Run.
type RunOptions struct {
	Parallel   bool
	Args       []string  // extra args passed to the task after "--"
	Prefer     []string  // scripts.prefer: which config file wins a name clash
	Executable string    // binary re-executed per project; "" = os.Executable()
	Runner     Runner    // nil = ExecRunner
	Stdout     io.Writer // nil = os.Stdout
	Stderr     io.Writer // nil = os.Stderr
}

// Run runs task in every project that defines it by re-executing xpm as
// `<exe> run -- <task>`, followed by `-- <opts.Args...>` when there are any,
// with the project as working directory. Projects without
// the task are skipped with a note on stderr; it is an error when no project
// has it. A directory listed by several ecosystems runs once. Failures are
// returned joined, in project order.
func Run(workspaces []Workspace, task string, opts RunOptions) error {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	exe := opts.Executable
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return fmt.Errorf("cannot locate the xpm binary: %w", err)
		}
	}
	runArgs := []string{"run", "--", task}
	if len(opts.Args) > 0 {
		runArgs = append(append(runArgs, "--"), opts.Args...)
	}
	seen := map[string]bool{}
	var steps []step
	for _, ws := range workspaces {
		for _, p := range ws.Projects {
			if seen[p.Path] {
				continue
			}
			seen[p.Path] = true
			merged, err := scripts.LoadAllScripts(p.Path, opts.Prefer)
			if err != nil {
				steps = append(steps, step{label: p.Name, err: fmt.Errorf("loading tasks: %w", err)})
				continue
			}
			if _, found := merged.GetScript(task); !found {
				_, _ = fmt.Fprintf(stderr, "[%s] skipped: no task %q\n", p.Name, task)
				continue
			}
			steps = append(steps, step{label: p.Name, cmd: Command{Dir: p.Path, Name: exe, Args: slices.Clone(runArgs)}})
		}
	}
	if len(steps) == 0 {
		return fmt.Errorf("no workspace project defines task %q", task)
	}
	return execute(context.Background(), steps, execOptions{
		parallel: opts.Parallel, run: opts.Runner, stdout: opts.Stdout, stderr: stderr,
	})
}

package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/crenspire/xpm/internal/pm"
)

// InstallOptions configures Install.
type InstallOptions struct {
	Parallel bool
	Runner   Runner                            // nil = ExecRunner
	LookPath func(file string) (string, error) // nil = exec.LookPath
	Stdout   io.Writer                         // nil = os.Stdout
	Stderr   io.Writer                         // nil = os.Stderr
}

// Install installs the dependencies of every workspace, in order. A workspace
// with a RootPM (npm/yarn/pnpm/bun, Cargo, Maven) is installed once at its
// root; every other project is installed in its own directory. Nothing is
// built. A project whose tool is not on PATH fails on its own; the others
// still run, and all failures are returned joined.
func Install(workspaces []Workspace, opts InstallOptions) error {
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	var steps []step
	for _, ws := range workspaces {
		if len(ws.Projects) == 0 {
			continue
		}
		if ws.RootPM != "" {
			label := ws.Ecosystem + " workspace"
			cmd, ok := installCommand(ws.RootPM, ws.Root)
			if !ok {
				_, _ = fmt.Fprintf(stderr, "[%s] skipped: no install command for %s\n", label, ws.RootPM)
				continue
			}
			steps = append(steps, checkTool(step{label: label, cmd: cmd}, opts.LookPath))
			continue
		}
		for _, p := range ws.Projects {
			cmd, ok := installCommand(p.PM, p.Path)
			if !ok {
				_, _ = fmt.Fprintf(stderr, "[%s] skipped: nothing to install for %s\n", p.Name, p.PM)
				continue
			}
			steps = append(steps, checkTool(step{label: p.Name, cmd: cmd}, opts.LookPath))
		}
	}
	if len(steps) == 0 {
		return errors.New("no workspace projects to install")
	}
	return execute(context.Background(), steps, execOptions{
		parallel: opts.Parallel, run: opts.Runner, stdout: opts.Stdout, stderr: stderr,
	})
}

// checkTool turns a step whose executable is not on PATH into a failed step.
func checkTool(s step, lookPath func(string) (string, error)) step {
	if _, err := lookPath(s.cmd.Name); err != nil {
		s.err = fmt.Errorf("%s is not installed (needed for: %s)", s.cmd.Name, s.cmd)
	}
	return s
}

// installCommand returns the command that downloads id's dependencies for
// the project or workspace root in dir without building anything. ok is
// false when there is nothing to run (a pip project without
// requirements.txt, or an unknown manager).
func installCommand(id pm.ID, dir string) (cmd Command, ok bool) {
	c := func(name string, args ...string) (Command, bool) {
		return Command{Dir: dir, Name: name, Args: args}, true
	}
	switch id {
	case pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun:
		meta, _ := pm.MetaFor(id)
		return c(meta.Binary, "install")
	case uvID:
		return c("uv", "sync")
	case pm.Poetry:
		return c("poetry", "install")
	case pm.Pipenv:
		return c("pipenv", "install")
	case pm.Pip:
		if !isFile(filepath.Join(dir, "requirements.txt")) {
			return Command{}, false
		}
		return c("pip", "install", "-r", "requirements.txt")
	case pm.Composer:
		return c("composer", "install")
	case pm.Cargo:
		return c("cargo", "fetch")
	case pm.GoMod:
		// GOWORK=off: download this module's own requirements even when a
		// go.work encloses it.
		cmd := Command{Dir: dir, Name: "go", Args: []string{"mod", "download"}, Env: []string{"GOWORK=off"}}
		return cmd, true
	case pm.Maven:
		return c("mvn", "-q", "dependency:resolve")
	case pm.Gradle:
		return c("gradle", "-q", "dependencies")
	}
	return Command{}, false
}

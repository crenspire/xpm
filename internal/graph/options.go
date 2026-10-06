package graph

import (
	"fmt"
	"os/exec"
)

// ExtractOptions controls how dependency graphs are extracted.
type ExtractOptions struct {
	// Exec allows running build tools (mvn, gradle, go) to resolve full
	// trees. Without it extractors only parse files.
	Exec bool
	// Run runs name with args in dir and returns its stdout. nil runs the
	// real command (stdout only; stderr is discarded). Tests inject fakes.
	Run func(dir, name string, args ...string) ([]byte, error)
	// Warn receives non-fatal warnings. nil discards them.
	Warn func(msg string)
}

// run runs a command through Run, or for real with cmd.Dir = dir.
func (o ExtractOptions) run(dir, name string, args ...string) ([]byte, error) {
	if o.Run != nil {
		return o.Run(dir, name, args...)
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// warn formats a warning and passes it to Warn, if set.
func (o ExtractOptions) warn(format string, a ...any) {
	if o.Warn != nil {
		o.Warn(fmt.Sprintf(format, a...))
	}
}

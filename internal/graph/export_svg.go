package graph

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ErrGraphVizNotFound is returned by WriteSVG when GraphViz's dot is not on PATH.
var ErrGraphVizNotFound = errors.New("GraphViz is not installed (no `dot` in PATH); install it from https://graphviz.org/download/ to use --svg")

// dotCommand returns the command that turns DOT on stdin into SVG on stdout.
// Tests replace it with a fake.
var dotCommand = func() (*exec.Cmd, error) {
	path, err := exec.LookPath("dot")
	if err != nil {
		return nil, ErrGraphVizNotFound
	}
	return exec.Command(path, "-Tsvg"), nil
}

// WriteSVG renders the graph as SVG by piping its DOT form into `dot -Tsvg`
// and streaming dot's stdout to w. dot's stderr is included in the error.
func WriteSVG(graph *DepGraph, w io.Writer) error {
	cmd, err := dotCommand()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stdin = strings.NewReader(ToDOT(graph))
	cmd.Stdout = w
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("dot -Tsvg failed: %w: %s", err, msg)
		}
		return fmt.Errorf("dot -Tsvg failed: %w", err)
	}
	return nil
}

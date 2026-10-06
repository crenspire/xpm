package graph

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// helperEnv makes the test binary act as a fake external command (see
// helperMain) instead of running the tests. This works on every OS, unlike
// shell-script fakes.
const helperEnv = "XPM_GRAPH_TEST_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(helperMain(mode))
	}
	os.Exit(m.Run())
}

// helperMain implements the fake commands:
//   - "pwd": print the working directory to stdout and noise to stderr.
//   - "dot": a fake `dot -Tsvg`; wraps stdin in <svg>...</svg> on stdout.
//   - "dot-fail": print a dot-style syntax error to stderr and exit 1.
func helperMain(mode string) int {
	switch mode {
	case "dot":
		in, err := io.ReadAll(os.Stdin)
		if err != nil || !strings.HasPrefix(string(in), "digraph ") {
			fmt.Fprintf(os.Stderr, "fake dot: bad input %q\n", in)
			return 1
		}
		fmt.Printf("<svg args=%q>\n%s</svg>\n", strings.Join(os.Args[1:], " "), in)
		return 0
	case "dot-fail":
		_, _ = io.ReadAll(os.Stdin)
		fmt.Fprintln(os.Stderr, "Error: <stdin>: syntax error in line 1 near 'x'")
		return 1
	case "pwd":
		wd, err := os.Getwd()
		if err != nil {
			return 3
		}
		fmt.Print(wd)
		fmt.Fprint(os.Stderr, "stderr noise")
		return 0
	}
	fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
	return 2
}

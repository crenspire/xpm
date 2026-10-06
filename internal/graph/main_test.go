package graph

import (
	"fmt"
	"os"
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
func helperMain(mode string) int {
	switch mode {
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

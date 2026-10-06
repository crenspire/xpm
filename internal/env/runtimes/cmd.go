package runtimes

import (
	"context"
	"os"
	"os/exec"
)

// runCmd runs a helper tool (rustup, brew) with extra environment, its
// output going to the user. A seam: tests never run real tools.
var runCmd = func(ctx context.Context, name string, args, extraEnv []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

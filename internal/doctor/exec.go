package doctor

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// auditTimeout bounds one audit tool run (audits query the network).
const auditTimeout = 2 * time.Minute

// auditWaitDelay bounds how long runCommand waits, after the tool exits or
// is killed at the timeout, for children of the tool that still hold its
// output open. A var so tests can shorten it.
var auditWaitDelay = 5 * time.Second

// lookPath finds an executable on PATH. Tests replace it.
var lookPath = exec.LookPath

// runCommand runs name with args in dir and returns its stdout and exit code.
// A non-zero exit is not an error (audit tools exit 1 when they find
// vulnerabilities); err is set only when the command could not be run or
// timed out, or when the tool's children kept its output open past
// auditWaitDelay (the output may be incomplete). Stderr is discarded so
// warnings never corrupt JSON output. Tests replace it.
var runCommand = func(dir, name string, args ...string) (stdout []byte, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), auditTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.WaitDelay = auditWaitDelay
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if ctx.Err() != nil {
		return out.Bytes(), -1, ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out.Bytes(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return out.Bytes(), -1, err
	}
	return out.Bytes(), 0, nil
}

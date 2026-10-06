package doctor

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// A grandchild that keeps stdout open must not hold runCommand past the
// wait delay once the tool itself has exited.
func TestRunCommandDoesNotWaitForGrandchildrenHoldingStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh and sleep")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found")
	}
	old := auditWaitDelay
	auditWaitDelay = 200 * time.Millisecond
	t.Cleanup(func() { auditWaitDelay = old })

	start := time.Now()
	_, _, err := runCommand(t.TempDir(), "sh", "-c", "sleep 10 & echo started")
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("runCommand took %v; the wait delay did not bound it", elapsed)
	}
	if err == nil {
		t.Error("err = nil, want an error: the output may be incomplete")
	}
}

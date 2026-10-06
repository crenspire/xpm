package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// recorder is a fake Runner that records every command, prints
// "<label>-1" and "<label>-2" lines (yielding in between, to provoke
// interleaving) and fails for the directories in fail.
type recorder struct {
	mu   sync.Mutex
	cmds []Command
	fail map[string]bool
}

func (r *recorder) run(_ context.Context, c Command, stdout, _ io.Writer) error {
	r.mu.Lock()
	r.cmds = append(r.cmds, c)
	r.mu.Unlock()
	name := filepath.Base(c.Dir)
	_, _ = fmt.Fprintf(stdout, "%s-1\n", name)
	runtime.Gosched() // let other projects write in between
	_, _ = fmt.Fprintf(stdout, "%s-2\n", name)
	if r.fail[c.Dir] {
		return errors.New("exit status 1")
	}
	return nil
}

// lines renders recorded commands as "<dir relative to root>: <command>",
// in the order they ran.
func (r *recorder) lines(root string) []string {
	var out []string
	for _, c := range r.cmds {
		out = append(out, relSlash(root, c.Dir)+": "+c.String())
	}
	return out
}

func TestNoChdirInWorkspacePackage(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "os.Chdir") {
			t.Errorf("%s calls os.Chdir; use Command.Dir", f)
		}
	}
}

func TestExecuteParallelDoesNotInterleave(t *testing.T) {
	rec := &recorder{}
	var steps []step
	for _, n := range []string{"a", "b", "c", "d"} {
		steps = append(steps, step{label: n, cmd: Command{Dir: filepath.Join("/w", n), Name: "x"}})
	}
	var out strings.Builder
	if err := execute(context.Background(), steps, execOptions{parallel: true, run: rec.run, stdout: &out, stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b", "c", "d"} {
		block := fmt.Sprintf("[%s] $ x\n%s-1\n%s-2\n", n, n, n)
		if !strings.Contains(out.String(), block) {
			t.Errorf("output lacks the contiguous block %q:\n%s", block, out.String())
		}
	}
}

func TestExecuteStdinOnlyForSequential(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		var mu sync.Mutex
		var got []io.Reader
		run := func(_ context.Context, c Command, _, _ io.Writer) error {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, c.Stdin)
			return nil
		}
		steps := []step{{label: "a", cmd: Command{Dir: "/w/a", Name: "x"}}, {label: "b", cmd: Command{Dir: "/w/b", Name: "x"}}}
		if err := execute(context.Background(), steps, execOptions{parallel: parallel, run: run, stdout: io.Discard, stderr: io.Discard}); err != nil {
			t.Fatal(err)
		}
		for _, in := range got {
			if (in != nil) == parallel {
				t.Errorf("parallel=%v: stdin = %v, want non-nil only when sequential", parallel, in)
			}
		}
		if len(got) != 2 {
			t.Errorf("parallel=%v: ran %d steps, want 2", parallel, len(got))
		}
	}
}

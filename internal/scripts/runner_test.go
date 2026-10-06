package scripts

import (
	"bytes"
	"os/exec"
	"reflect"
	"runtime"
	"testing"
)

func TestShellBuildersPassExtraArgsAsPositionalParameters(t *testing.T) {
	builders := map[string]func(ScriptDefinition, []string) *exec.Cmd{
		"python": buildPythonCommand,
		"cargo":  buildCargoCommand,
		"shell":  buildShellCommand,
	}
	extra := []string{"a b", "$(echo pwned)", "c;d"}
	script := ScriptDefinition{Command: "echo hi"}

	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			got := build(script, extra).Args
			want := []string{"sh", "-c", script.Command + ` "$@"`, "sh", "a b", "$(echo pwned)", "c;d"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Args with extras = %q, want %q", got, want)
			}

			got = build(script, nil).Args
			want = []string{"sh", "-c", script.Command}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Args without extras = %q, want %q", got, want)
			}
		})
	}
}

func TestBuildShellCommandDoesNotEvaluateExtraArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found")
	}
	cmd := buildShellCommand(ScriptDefinition{Command: "printf '%s\\n'"}, []string{"a b", "$(echo pwned)", "c;d"})
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "a b\n$(echo pwned)\nc;d\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestShellCommandTrimsTrailingWhitespaceBeforeAppendingArgs(t *testing.T) {
	got := shellCommand("echo hi\n", []string{"a"}).Args
	want := []string{"sh", "-c", `echo hi "$@"`, "sh", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Args = %q, want %q", got, want)
	}
	if got := shellCommand("echo hi\n", nil).Args; got[2] != "echo hi\n" {
		t.Errorf("without extras the command must be left alone, got %q", got[2])
	}
}

func TestShellCommandMultilineScriptWithTrailingNewlineReceivesArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found")
	}
	cmd := buildShellCommand(ScriptDefinition{Command: "printf '%s\\n'\n"}, []string{"a"})
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v (%q)", err, out.String())
	}
	if want := "a\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

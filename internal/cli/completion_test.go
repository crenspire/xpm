package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var completionShells = []string{"bash", "zsh", "fish"}

func completionScript(t *testing.T, shell string) string {
	t.Helper()
	var sb strings.Builder
	if err := writeCompletion(&sb, shell); err != nil {
		t.Fatalf("writeCompletion(%s): %v", shell, err)
	}
	return sb.String()
}

func TestCompletionScriptsListEveryCommand(t *testing.T) {
	for _, shell := range completionShells {
		script := completionScript(t, shell)
		for _, w := range completionWords() {
			if !strings.Contains(script, w) {
				t.Errorf("%s script is missing %q", shell, w)
			}
		}
	}
}

func TestCompletionWordsSkipDashAndDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, w := range completionWords() {
		if strings.HasPrefix(w, "-") {
			t.Errorf("word %q starts with -", w)
		}
		if seen[w] {
			t.Errorf("duplicate word %q", w)
		}
		seen[w] = true
	}
	for _, want := range []string{"install", "i", "completion", "version"} {
		if !seen[want] {
			t.Errorf("missing word %q", want)
		}
	}
}

func TestCompletionFlagsKeysAreCommands(t *testing.T) {
	names := map[string]bool{}
	for _, c := range commandTable {
		names[c.name] = true
	}
	for k := range completionFlags {
		if !names[k] {
			t.Errorf("completionFlags key %q is not a canonical command", k)
		}
	}
}

func TestCompletionUnsupportedShell(t *testing.T) {
	var sb strings.Builder
	err := writeCompletion(&sb, "powershell")
	if err == nil || !strings.Contains(err.Error(), `unsupported shell "powershell" (want bash, zsh or fish)`) {
		t.Errorf("err = %v", err)
	}
}

func checkShellSyntax(t *testing.T, shell string, args ...string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell syntax check not run on windows")
	}
	path, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("%s not on PATH", shell)
	}
	file := filepath.Join(t.TempDir(), "xpm-completion."+shell)
	if err := os.WriteFile(file, []byte(completionScript(t, shell)), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(path, append(args, file)...).CombinedOutput()
	if err != nil {
		t.Errorf("%s syntax check failed: %v\n%s", shell, err, out)
	}
}

func TestCompletionBashSyntax(t *testing.T) { checkShellSyntax(t, "bash", "-n") }
func TestCompletionZshSyntax(t *testing.T)  { checkShellSyntax(t, "zsh", "-n") }
func TestCompletionFishSyntax(t *testing.T) { checkShellSyntax(t, "fish", "--no-execute") }

func TestCompletionBashCompletes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash completion check not run on windows")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	file := filepath.Join(t.TempDir(), "xpm.bash")
	if err := os.WriteFile(file, []byte(completionScript(t, "bash")), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ words, cword, want string }{
		{"xpm ins", "1", "install"},
		{"xpm i --w", "2", "--workspace"},
		{"xpm man gr", "2", "graph"},
		{"xpm completion z", "2", "zsh"},
		{"xpm config s", "2", "show"},
		{"xpm env ls", "2", "ls-remote"},
		{"xpm lock --v", "2", "--verify"},
	}
	for _, c := range cases {
		script := "source " + file + "; COMP_WORDS=(" + c.words + "); COMP_CWORD=" + c.cword + "; _xpm; echo \"${COMPREPLY[@]}\""
		out, err := exec.Command(bash, "--norc", "--noprofile", "-c", script).CombinedOutput()
		if err != nil {
			t.Fatalf("%q: %v\n%s", c.words, err, out)
		}
		found := false
		for _, f := range strings.Fields(string(out)) {
			if f == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: got %q, want it to contain %q", c.words, out, c.want)
		}
	}
}

func TestCompletionCmdPrintsScript(t *testing.T) {
	var code int
	out := captureStdout(t, func() { code = cmdCompletion([]string{"bash"}) })
	if code != 0 || !strings.Contains(out, "complete -o default -F _xpm xpm") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestCompletionUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"bash", "zsh"}, {"powershell"}} {
		var code int
		_ = captureStdout(t, func() {
			_ = captureStderr(t, func() { code = cmdCompletion(args) })
		})
		if code != 2 {
			t.Errorf("args %v: code = %d, want 2", args, code)
		}
	}
}

package pm

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// fakeTools makes exactly the binaries in onPath "exist" and records every
// command InstallPM runs. Running a command named in `installs` puts that
// binary on PATH afterwards.
func fakeTools(t *testing.T, onPath []string, installs map[string]string) *[]string {
	t.Helper()
	path := map[string]bool{}
	for _, b := range onPath {
		path[b] = true
	}
	var ran []string
	restoreRun := SetCommandRunner(func(bin string, args ...string) error {
		cmd := bin + " " + strings.Join(args, " ")
		ran = append(ran, cmd)
		if b, ok := installs[cmd]; ok {
			path[b] = true
		}
		return nil
	})
	restoreLook := SetLookPath(func(file string) (string, error) {
		if path[file] {
			return "/usr/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	})
	t.Cleanup(func() { restoreRun(); restoreLook() })
	return &ran
}

func TestInstallPMNeverRunsRemoteScripts(t *testing.T) {
	ran := fakeTools(t, []string{"curl", "sh", "bash", "php"}, nil)
	for id, url := range map[ID]string{Bun: "https://bun.sh", Cargo: "https://rustup.rs", Composer: "https://getcomposer.org"} {
		err := InstallPM(id)
		var manual *ManualInstallError
		if !errors.As(err, &manual) || !strings.Contains(manual.Steps, url) {
			t.Errorf("InstallPM(%s) = %v, want a ManualInstallError pointing at %s", id, err, url)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %v; nothing may be executed for script-installed tools", *ran)
	}
}

func TestInstallPMRechecksPath(t *testing.T) {
	ran := fakeTools(t, []string{"npm"}, nil) // npm "succeeds" but pnpm never appears
	err := InstallPM(Pnpm)
	if err == nil || !strings.Contains(err.Error(), "not on PATH") {
		t.Fatalf("err = %v, want a not-on-PATH error", err)
	}
	if len(*ran) != 1 || (*ran)[0] != "npm install -g pnpm" {
		t.Fatalf("ran %v", *ran)
	}
}

func TestInstallPMSucceedsWhenBinaryAppears(t *testing.T) {
	fakeTools(t, []string{"npm"}, map[string]string{"npm install -g yarn": "yarn"})
	if err := InstallPM(Yarn); err != nil {
		t.Fatal(err)
	}
}

func TestInstallPMFallsBackToTheNextInstaller(t *testing.T) {
	ran := fakeTools(t, []string{"python"}, map[string]string{"python -m ensurepip --upgrade --default-pip": "pip"})
	if err := InstallPM(Pip); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 1 || (*ran)[0] != "python -m ensurepip --upgrade --default-pip" {
		t.Fatalf("ran %v, want only the python fallback (python3 is missing)", *ran)
	}
}

func TestInstallPMWithoutAnyInstallerFails(t *testing.T) {
	ran := fakeTools(t, nil, nil)
	if err := InstallPM(Poetry); err == nil || !strings.Contains(err.Error(), "pipx is not installed") {
		t.Fatalf("err = %v", err)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %v", *ran)
	}
}

func TestInstallHintsNeverPipeToAShell(t *testing.T) {
	for _, m := range AllMetas() {
		if h := InstallHint(m.ID); strings.Contains(h, "| sh") || strings.Contains(h, "| bash") {
			t.Errorf("InstallHint(%s) = %q suggests piping a download into a shell", m.ID, h)
		}
	}
}

func TestGoModulesSupportGlobalInstalls(t *testing.T) {
	meta, _ := MetaFor(GoMod)
	if !meta.SupportsGlobal {
		t.Fatal("go modules: -g runs go install, so SupportsGlobal must be true")
	}
	var ran []string
	t.Cleanup(SetCommandRunner(func(bin string, args ...string) error {
		ran = append(ran, bin+" "+strings.Join(args, " "))
		return nil
	}))
	if err := (GoModAdapter{}).InstallPackage("golang.org/x/tools/gopls", true, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || ran[0] != "go install golang.org/x/tools/gopls@latest" {
		t.Fatalf("ran %q", ran)
	}
}

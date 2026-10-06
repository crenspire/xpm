package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

func TestEffectiveInteractive(t *testing.T) {
	if effectiveInteractive(true, false) || effectiveInteractive(false, true) || !effectiveInteractive(true, true) {
		t.Fatal("prompts need both the config and a terminal")
	}
}

func TestRunWithoutATerminalIsNonInteractive(t *testing.T) {
	withConfig(t, cfg)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	oldArgs, oldTTY := os.Args, isInteractiveTerminal
	t.Cleanup(func() { os.Args, isInteractiveTerminal = oldArgs, oldTTY })
	os.Args = []string{"xpm", "version"}

	for _, tty := range []bool{false, true} {
		isInteractiveTerminal = func() bool { return tty }
		captureStdout(t, func() { Run() })
		if cfg.Interactive != tty {
			t.Errorf("tty=%v: cfg.Interactive = %v (default config says interactive)", tty, cfg.Interactive)
		}
	}
}

func TestNonInteractiveRefusesToGuessWhenARegistryWasDown(t *testing.T) {
	if err := refuseGuessWhenUnavailable(nil); err != nil {
		t.Fatalf("all registries answered: err = %v", err)
	}
	err := refuseGuessWhenUnavailable([]pm.ID{pm.Maven})
	if err == nil || !strings.Contains(err.Error(), "maven (Java) did not answer") {
		t.Fatalf("err = %v", err)
	}
}

func TestChooseCandidateNonInteractiveWithUnavailableRegistry(t *testing.T) {
	withConfig(t, config.Config{Interactive: false})
	// Same-ecosystem candidates would normally be auto-picked.
	cands := []candidate{{Result: npmAxios}}
	if _, ok := chooseCandidate(cands, "axios", nil, false, []pm.ID{pm.Maven}); ok {
		t.Fatal("must refuse to pick while maven was down")
	}
	if c, ok := chooseCandidate(cands, "axios", nil, false, nil); !ok || c.Result.Manager != pm.Npm {
		t.Fatalf("all answered: got (%+v, %v)", c, ok)
	}
}

func TestInstallWithoutTerminalAndMissingRegistryExitsOne(t *testing.T) {
	withConfig(t, config.Config{Interactive: false})
	inProject(t) // no project file: nothing settles the choice
	ran := recordTools(t)
	withLookupReport(t, search.Report{
		Results:     []search.Result{npmAxios},
		Unavailable: []search.RegistryFailure{timedOut},
	}, nil)
	var code int
	captureStdout(t, func() { code = cmdInstall([]string{"axios"}) })
	if code != 1 || len(*ran) != 0 {
		t.Fatalf("code=%d ran=%v: must not install a guess while maven was down", code, *ran)
	}
}

func TestInstallWithoutTerminalUsesTheProjectsEcosystemWhenAnotherRegistryIsDown(t *testing.T) {
	withConfig(t, config.Config{Interactive: false})
	inProject(t, "package.json", "package-lock.json")
	ran := recordAllCommands(t)
	withLookupReport(t, search.Report{
		Results:     []search.Result{npmAxios},
		Unavailable: []search.RegistryFailure{timedOut},
	}, nil)
	var code int
	captureStdout(t, func() { code = cmdInstall([]string{"axios"}) })
	if code != 0 || len(*ran) != 1 || (*ran)[0] != "npm install axios" {
		t.Fatalf("code=%d ran=%q: a maven outage cannot change a Node project's npm hit", code, *ran)
	}
}

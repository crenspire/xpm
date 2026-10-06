package cli

import (
	"reflect"
	"testing"

	"github.com/crenspire/xpm/internal/config"
)

func TestParseInstallArgs(t *testing.T) {
	for _, c := range []struct {
		args    []string
		global  bool
		pkgs    []string
		wantErr bool
		ws      bool
	}{
		{[]string{"axios"}, false, []string{"axios"}, false, false},
		{[]string{"axios", "-g"}, true, []string{"axios"}, false, false},
		{[]string{"-g", "typescript"}, true, []string{"typescript"}, false, false},
		{[]string{"a", "--global", "b", "c"}, true, []string{"a", "b", "c"}, false, false},
		{[]string{"axios", "--save-dev"}, false, nil, true, false},
		{[]string{"--", "-weird"}, false, []string{"-weird"}, false, false},
		{nil, false, nil, false, false},
		{[]string{"-w"}, false, nil, false, true},
		{[]string{"--workspace"}, false, nil, false, true},
		{[]string{"-workspace"}, false, nil, false, true},
		{[]string{"-w", "-g"}, true, nil, false, true},
		{[]string{"--workspace=x"}, false, nil, true, false},
		{[]string{"-w=x"}, false, nil, true, false},
		{[]string{"axios", "-w"}, false, []string{"axios"}, false, true},
		{[]string{"-w", "--", "-w"}, false, []string{"-w"}, false, true},
	} {
		got, err := parseInstallArgs(c.args)
		if (err != nil) != c.wantErr {
			t.Errorf("%v: err = %v, wantErr %v", c.args, err, c.wantErr)
			continue
		}
		if !c.wantErr && (got.Global != c.global || got.Workspace != c.ws || !reflect.DeepEqual(got.Packages, c.pkgs)) {
			t.Errorf("%v: got %+v, want global=%v ws=%v pkgs=%v", c.args, got, c.global, c.ws, c.pkgs)
		}
	}
}

func TestInstallsEveryPackageInOrderAndStopsAtFirstFailure(t *testing.T) {
	withConfig(t, config.Config{})
	var seen []string
	withInstallOne(t, func(spec string, global bool) int {
		seen = append(seen, spec)
		if spec == "b" {
			return 1
		}
		return 0
	})
	var code int
	captureStdout(t, func() { code = cmdInstall([]string{"a", "b", "c", "-g"}) })
	if code != 1 || !reflect.DeepEqual(seen, []string{"a", "b"}) {
		t.Fatalf("code=%d seen=%v, want 1 and [a b]", code, seen)
	}
}

func TestExtraArgumentsAreAnError(t *testing.T) {
	withConfig(t, config.Config{})
	for name, run := range map[string]func() int{
		"which":  func() int { return cmdWhich([]string{"axios", "lodash"}) },
		"info":   func() int { return cmdInfo([]string{"axios", "lodash"}) },
		"remove": func() int { return cmdRemove([]string{"axios", "lodash"}) },
		"update": func() int { return cmdUpdate([]string{"axios", "lodash"}) },
		"list":   func() int { return cmdList([]string{"axios"}) },
	} {
		t.Run(name, func(t *testing.T) {
			var code int
			captureStdout(t, func() { code = run() })
			if code != 1 {
				t.Errorf("an extra argument exited %d, want 1", code)
			}
		})
	}
}

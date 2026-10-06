package search

import (
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
)

func TestOptionsFromConfig(t *testing.T) {
	var c config.Config
	c.Search = map[string]bool{"maven": false, "gomod": true}
	c.Timeout.Default = 3
	c.Timeout.PerRegistry = map[string]int{"crates": 1, "PyPI": 5, "bogus": 9, "npm": 0}

	o := OptionsFromConfig(c)
	if Enabled(o, pm.Maven) || !Enabled(o, pm.Npm) || !Enabled(o, pm.Cargo) {
		t.Fatalf("Enable = %v, want maven off and the rest on", o.Enable)
	}
	for id, want := range map[pm.ID]time.Duration{
		pm.Cargo: time.Second, pm.Pip: 5 * time.Second, pm.Npm: 3 * time.Second, pm.Maven: 3 * time.Second,
	} {
		if got := o.timeoutFor(id); got != want {
			t.Errorf("timeoutFor(%s) = %v, want %v", id, got, want)
		}
	}
}

func TestOptionsFromEmptyConfigUseTheBuiltInDeadline(t *testing.T) {
	o := OptionsFromConfig(config.Config{})
	if got := o.timeoutFor(pm.Npm); got != lookupDeadline {
		t.Fatalf("timeoutFor = %v, want lookupDeadline %v", got, lookupDeadline)
	}
	for _, id := range Registries {
		if !Enabled(o, id) {
			t.Errorf("%s disabled by an empty config", id)
		}
	}
}

func TestOptionsFromConfigTreatsLegacy4AsDefault(t *testing.T) {
	o := OptionsFromConfig(config.Config{Timeout: config.TimeoutConfig{Default: 4}})
	if o.Timeout != 0 {
		t.Fatalf("Timeout = %v, want 0 (legacy 4 means built-in deadline)", o.Timeout)
	}
	if got := o.timeoutFor(pm.Npm); got != lookupDeadline {
		t.Fatalf("timeoutFor = %v, want lookupDeadline %v", got, lookupDeadline)
	}
}

func TestOptionsFromConfigHonoursOtherDefaults(t *testing.T) {
	for _, secs := range []int{3, 6} {
		o := OptionsFromConfig(config.Config{Timeout: config.TimeoutConfig{Default: secs}})
		if want := time.Duration(secs) * time.Second; o.Timeout != want {
			t.Errorf("Default %d: Timeout = %v, want %v", secs, o.Timeout, want)
		}
	}
}

func TestPerRegistryTimeoutCutsOnlyThatRegistry(t *testing.T) {
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 150*time.Millisecond, nil),
		fakeLookup(pm.Maven, 5*time.Second, nil),
	})
	opts := Options{Timeout: 400 * time.Millisecond, RegistryTimeout: map[pm.ID]time.Duration{pm.Maven: 50 * time.Millisecond}}
	start := time.Now()
	rep, err := SearchEverywhereReport("x", opts)
	if d := time.Since(start); d > 350*time.Millisecond {
		t.Fatalf("took %v; npm answered at 150 ms and maven was cut at 50 ms", d)
	}
	if err != nil || len(rep.Results) != 1 || rep.Results[0].Manager != pm.Npm {
		t.Fatalf("got (%+v, %v), want the npm result", rep, err)
	}
	if len(rep.Unavailable) != 1 || !rep.Unavailable[0].TimedOut() {
		t.Fatalf("Unavailable = %+v, want maven timed out", rep.Unavailable)
	}
}

func TestConfiguredTimeoutCanExceedTheDefaultDeadline(t *testing.T) {
	withDeadline(t, 50*time.Millisecond)
	withLookups(t, []lookup{fakeLookup(pm.Npm, 150*time.Millisecond, nil)})
	rep, err := SearchEverywhereReport("x", Options{Timeout: 500 * time.Millisecond})
	if err != nil || len(rep.Results) != 1 {
		t.Fatalf("a slow-network user who raised timeout.default must get the result: (%+v, %v)", rep, err)
	}
}

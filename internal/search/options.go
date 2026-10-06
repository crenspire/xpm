package search

import (
	"strings"
	"time"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
)

// Registries are the registries xpm searches, in result order.
var Registries = []pm.ID{pm.Npm, pm.Pip, pm.Composer, pm.Cargo, pm.Maven}

// registryAliases maps timeout.perRegistry keys to registries. Both the
// registry names documented in config.TimeoutConfig and manager IDs work.
var registryAliases = map[string]pm.ID{
	"npm": pm.Npm, "pypi": pm.Pip, "pip": pm.Pip,
	"packagist": pm.Composer, "composer": pm.Composer,
	"crates": pm.Cargo, "cargo": pm.Cargo, "maven": pm.Maven,
}

// legacyDefaultTimeoutSecs is the pre-P3 timeout.default value.
const legacyDefaultTimeoutSecs = 4

// OptionsFromConfig builds search options from the user's config: which
// registries are enabled (search.<id>, default true) and how long each may
// take (timeout.default / timeout.perRegistry, in seconds; 0 or absent means
// the built-in 2.5 s deadline).
func OptionsFromConfig(c config.Config) Options {
	opts := Options{Enable: make(map[pm.ID]bool, len(Registries))}
	for _, id := range Registries {
		enabled := true
		if v, ok := c.Search[string(id)]; ok {
			enabled = v
		}
		opts.Enable[id] = enabled
	}
	// 4 was the old built-in default that `xpm config set` wrote into users'
	// files; treat it as "use the built-in deadline" so those configs don't
	// silently slow every lookup to 4 s (user decision 2026-10-07).
	if c.Timeout.Default > 0 && c.Timeout.Default != legacyDefaultTimeoutSecs {
		opts.Timeout = time.Duration(c.Timeout.Default) * time.Second
	}
	for key, secs := range c.Timeout.PerRegistry {
		id, ok := registryAliases[strings.ToLower(key)]
		if !ok || secs <= 0 {
			continue
		}
		if opts.RegistryTimeout == nil {
			opts.RegistryTimeout = make(map[pm.ID]time.Duration)
		}
		opts.RegistryTimeout[id] = time.Duration(secs) * time.Second
	}
	return opts
}

// timeoutFor is how long registry id may take: its own timeout, else the
// default timeout, else lookupDeadline.
func (o Options) timeoutFor(id pm.ID) time.Duration {
	if d := o.RegistryTimeout[id]; d > 0 {
		return d
	}
	if o.Timeout > 0 {
		return o.Timeout
	}
	return lookupDeadline
}

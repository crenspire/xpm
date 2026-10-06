package graph

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// yarnEntry is one resolved package of a yarn.lock (v1 or berry).
type yarnEntry struct {
	specs []string          // "name@range" keys that resolve to this entry
	name  string            // real package name
	ver   string            // resolved version
	deps  map[string]string // dependency name -> range as written in the lockfile
	root  bool              // berry "name@workspace:." entry (the project itself)
	ws    bool              // berry workspace package other than the root
}

// parseYarnLock builds the graph of a yarn.lock, classic v1 or berry (v2+).
// The root is the project: berry's "name@workspace:." entry, else
// package.json (manifest), whose dependencies become the project's edges;
// fallback names a project package.json does not name. Berry workspace
// packages hang off the project.
func parseYarnLock(data, manifest []byte, fallback string) (*DepGraph, error) {
	var entries []*yarnEntry
	var err error
	berry := false
	if bytes.Contains(data, []byte("\n__metadata:")) || bytes.HasPrefix(data, []byte("__metadata:")) {
		berry = true
		entries, err = parseYarnBerry(data)
	} else {
		entries, err = parseYarnV1(data)
	}
	if err != nil {
		return nil, err
	}

	g := NewGraph()
	bySpec := map[string]*yarnEntry{}
	ids := map[*yarnEntry]string{}
	var rootEntry *yarnEntry
	for _, e := range entries {
		for _, s := range e.specs {
			bySpec[s] = e
		}
		if e.root {
			rootEntry = e
			continue
		}
		n := NewDepNode("node", e.name, e.ver)
		if g.GetNode(n.ID) == nil {
			g.AddNode(n)
		}
		ids[e] = n.ID
	}
	lookup := func(name, rng string) (string, bool) {
		e, ok := bySpec[name+"@"+rng]
		if !ok && berry && !strings.Contains(rng, ":") {
			e, ok = bySpec[name+"@npm:"+rng]
		}
		if !ok {
			return "", false
		}
		id, ok := ids[e]
		return id, ok
	}
	for _, e := range entries {
		from, ok := ids[e]
		if !ok {
			continue
		}
		for _, kv := range sortedPairs(e.deps) {
			if to, ok := lookup(kv[0], kv[1]); ok {
				g.AddEdge(NewEdge(from, to))
			}
		}
	}

	var project string
	var rootDeps map[string]string
	if rootEntry != nil {
		project = addProject(g, "node", rootEntry.name, rootEntry.ver, fallback)
		rootDeps = rootEntry.deps
	} else {
		m, err := parseNpmManifest(manifest)
		if err != nil {
			return nil, err
		}
		project = addProject(g, "node", m.Name, m.Version, fallback)
		rootDeps = map[string]string{}
		for _, sec := range []map[string]string{m.PeerDependencies, m.DevDependencies, m.Dependencies, m.OptionalDependencies} {
			for k, v := range sec {
				rootDeps[k] = v
			}
		}
	}
	for _, kv := range sortedPairs(rootDeps) {
		if id, ok := lookup(kv[0], kv[1]); ok {
			g.AddEdge(NewEdge(project, id))
		}
	}
	for _, e := range entries {
		if e.ws {
			g.AddEdge(NewEdge(project, ids[e]))
		}
	}
	return g, nil
}

// yarnSplitSpec splits "name@range" at the first "@" after a scope "@":
// "@babel/core@^7.0.0" -> ("@babel/core", "^7.0.0"),
// "string-width-cjs@npm:string-width@^4.2.0" -> ("string-width-cjs", "npm:string-width@^4.2.0").
func yarnSplitSpec(spec string) (name, rng string, ok bool) {
	if len(spec) < 2 {
		return "", "", false
	}
	at := strings.IndexByte(spec[1:], '@')
	if at < 0 {
		return "", "", false
	}
	return spec[:at+1], spec[at+2:], true
}

// yarnRealName is the package an entry installs: for an npm alias
// ("alias@npm:real@^1") it is the real name, otherwise the spec's name.
func yarnRealName(name, rng string) string {
	if r, ok := strings.CutPrefix(rng, "npm:"); ok {
		if at := strings.LastIndexByte(r, '@'); at > 0 {
			return r[:at]
		}
	}
	return name
}

func yarnUnquote(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		return strconv.Unquote(s)
	}
	return s, nil
}

// parseYarnV1 reads the classic "# yarn lockfile v1" format.
func parseYarnV1(data []byte) ([]*yarnEntry, error) {
	var entries []*yarnEntry
	var cur *yarnEntry
	section := ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	finish := func() error {
		if cur != nil && cur.ver == "" {
			return fmt.Errorf("yarn.lock: entry %q has no version", strings.Join(cur.specs, ", "))
		}
		return nil
	}
	for sc.Scan() {
		lineNo++
		line := strings.TrimRight(sc.Text(), " \r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		switch {
		case indent == 0:
			if err := finish(); err != nil {
				return nil, err
			}
			if !strings.HasSuffix(line, ":") {
				return nil, fmt.Errorf("yarn.lock line %d: expected an entry header ending in ':'", lineNo)
			}
			cur = &yarnEntry{deps: map[string]string{}}
			section = ""
			for _, raw := range strings.Split(strings.TrimSuffix(line, ":"), ",") {
				spec, err := yarnUnquote(raw)
				if err != nil {
					return nil, fmt.Errorf("yarn.lock line %d: %w", lineNo, err)
				}
				name, rng, ok := yarnSplitSpec(spec)
				if !ok {
					return nil, fmt.Errorf("yarn.lock line %d: cannot parse %q", lineNo, spec)
				}
				cur.specs = append(cur.specs, spec)
				if cur.name == "" {
					cur.name = yarnRealName(name, rng)
				}
			}
			entries = append(entries, cur)
		case cur == nil:
			return nil, fmt.Errorf("yarn.lock line %d: indented line outside an entry", lineNo)
		case indent == 2:
			key, val, _ := strings.Cut(trimmed, " ")
			section = ""
			switch {
			case key == "version":
				v, err := yarnUnquote(val)
				if err != nil {
					return nil, fmt.Errorf("yarn.lock line %d: %w", lineNo, err)
				}
				cur.ver = v
			case strings.HasSuffix(trimmed, ":"):
				section = strings.TrimSuffix(trimmed, ":")
			}
		default:
			if section != "dependencies" && section != "optionalDependencies" {
				continue
			}
			k, v, ok := yarnSplitDepLine(trimmed)
			if !ok {
				return nil, fmt.Errorf("yarn.lock line %d: cannot parse dependency %q", lineNo, trimmed)
			}
			cur.deps[k] = v
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("yarn.lock: %w", err)
	}
	if err := finish(); err != nil {
		return nil, err
	}
	return entries, nil
}

// yarnSplitDepLine parses `name "range"` or `"@scope/name" "range"`.
func yarnSplitDepLine(s string) (name, rng string, ok bool) {
	var rest string
	if strings.HasPrefix(s, `"`) {
		end := strings.Index(s[1:], `"`)
		if end < 0 {
			return "", "", false
		}
		name, rest = s[1:end+1], s[end+2:]
	} else {
		name, rest, ok = strings.Cut(s, " ")
		if !ok {
			return "", "", false
		}
	}
	r, err := yarnUnquote(rest)
	if err != nil || name == "" || r == "" {
		return "", "", false
	}
	return name, r, true
}

// parseYarnBerry reads a Yarn 2+ lockfile (YAML with __metadata).
func parseYarnBerry(data []byte) ([]*yarnEntry, error) {
	var doc map[string]struct {
		Version      string            `yaml:"version"`
		Resolution   string            `yaml:"resolution"`
		Dependencies map[string]string `yaml:"dependencies"`
		Optional     map[string]string `yaml:"optionalDependencies"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("yarn.lock: %w", err)
	}
	var entries []*yarnEntry
	for _, key := range sortedKeys(doc) {
		if key == "__metadata" {
			continue
		}
		v := doc[key]
		name, rng, ok := yarnSplitSpec(v.Resolution)
		if !ok || v.Version == "" {
			return nil, fmt.Errorf("yarn.lock: entry %q has no resolution or version", key)
		}
		e := &yarnEntry{name: name, ver: v.Version, deps: map[string]string{}}
		for _, s := range strings.Split(key, ",") {
			e.specs = append(e.specs, strings.TrimSpace(s))
		}
		for k, r := range v.Dependencies {
			e.deps[k] = r
		}
		for k, r := range v.Optional {
			e.deps[k] = r
		}
		if strings.HasPrefix(rng, "workspace:") {
			e.root = rng == "workspace:."
			e.ws = !e.root
		}
		entries = append(entries, e)
	}
	return entries, nil
}

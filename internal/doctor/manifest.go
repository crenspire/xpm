package doctor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

// nameSet is a set of dependency names.
type nameSet map[string]bool

func (s nameSet) addKeys(m map[string]json.RawMessage) {
	for k := range m {
		s[k] = true
	}
}

// minus returns the sorted names in s that are not in other.
func (s nameSet) minus(other nameSet) []string {
	var out []string
	for k := range s {
		if !other[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// npmDepFields are the package.json fields npm records in the root entry of
// package-lock.json.
type npmDepFields struct {
	Dependencies         map[string]json.RawMessage `json:"dependencies"`
	DevDependencies      map[string]json.RawMessage `json:"devDependencies"`
	OptionalDependencies map[string]json.RawMessage `json:"optionalDependencies"`
	PeerDependencies     map[string]json.RawMessage `json:"peerDependencies"`
}

func (f npmDepFields) names(withPeers bool) nameSet {
	s := nameSet{}
	s.addKeys(f.Dependencies)
	s.addKeys(f.DevDependencies)
	s.addKeys(f.OptionalDependencies)
	if withPeers {
		s.addKeys(f.PeerDependencies)
	}
	return s
}

// readPackageJSON returns package.json's declared dependency fields.
func readPackageJSON(dir string) (npmDepFields, error) {
	var f npmDepFields
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("package.json: %w", err)
	}
	return f, nil
}

// npmLockRoot returns the root package entry (packages[""]) of a
// package-lock.json; ok is false for lockfileVersion 1 files, which have none.
func npmLockRoot(dir string) (root npmDepFields, ok bool, err error) {
	data, err := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if err != nil {
		return root, false, err
	}
	var lf struct {
		Packages map[string]npmDepFields `json:"packages"`
	}
	if err := json.Unmarshal(data, &lf); err != nil {
		return root, false, fmt.Errorf("package-lock.json: %w", err)
	}
	root, ok = lf.Packages[""]
	return root, ok, nil
}

// pnpmImporterNames returns the direct dependency names pnpm-lock.yaml records
// for the root project: importers["."] (workspaces and lockfile v9), or the
// top-level dependency maps of single-project v5/v6 lockfiles.
func pnpmImporterNames(dir string) (nameSet, error) {
	data, err := os.ReadFile(filepath.Join(dir, "pnpm-lock.yaml"))
	if err != nil {
		return nil, err
	}
	type depMaps struct {
		Dependencies         map[string]yaml.Node `yaml:"dependencies"`
		DevDependencies      map[string]yaml.Node `yaml:"devDependencies"`
		OptionalDependencies map[string]yaml.Node `yaml:"optionalDependencies"`
	}
	var lf struct {
		depMaps   `yaml:",inline"`
		Importers map[string]depMaps `yaml:"importers"`
	}
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("pnpm-lock.yaml: %w", err)
	}
	maps := lf.depMaps
	if root, ok := lf.Importers["."]; ok {
		maps = root
	}
	s := nameSet{}
	for _, m := range []map[string]yaml.Node{maps.Dependencies, maps.DevDependencies, maps.OptionalDependencies} {
		for k := range m {
			s[k] = true
		}
	}
	return s, nil
}

// cargoManifestNames returns the package names Cargo.toml depends on, from
// [dependencies], [dev-dependencies], [build-dependencies] and their
// [target.*] variants. A renamed dependency (`alias = { package = "real" }`)
// contributes its real package name. [workspace.dependencies] is not
// included: it only declares versions members may use.
func cargoManifestNames(dir string) (nameSet, error) {
	var m map[string]interface{}
	if _, err := toml.DecodeFile(filepath.Join(dir, "Cargo.toml"), &m); err != nil {
		return nil, fmt.Errorf("parse Cargo.toml: %w", err)
	}
	s := nameSet{}
	addTables := func(t map[string]interface{}) {
		for _, field := range []string{"dependencies", "dev-dependencies", "build-dependencies"} {
			deps, _ := t[field].(map[string]interface{})
			for key, spec := range deps {
				name := key
				if table, ok := spec.(map[string]interface{}); ok {
					if pkg, ok := table["package"].(string); ok && pkg != "" {
						name = pkg
					}
				}
				s[name] = true
			}
		}
	}
	addTables(m)
	targets, _ := m["target"].(map[string]interface{})
	for _, t := range targets {
		if table, ok := t.(map[string]interface{}); ok {
			addTables(table)
		}
	}
	return s, nil
}

// cargoLockNames returns the package names recorded in Cargo.lock.
func cargoLockNames(dir string) (nameSet, error) {
	var lf struct {
		Package []struct {
			Name string `toml:"name"`
		} `toml:"package"`
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "Cargo.lock"), &lf); err != nil {
		return nil, fmt.Errorf("parse Cargo.lock: %w", err)
	}
	s := nameSet{}
	for _, p := range lf.Package {
		s[p.Name] = true
	}
	return s, nil
}

// goRequirements returns "path version" for each go.mod requirement whose
// checksum must be in go.sum: requirements that a replace directive redirects
// (to a local directory or another module) are skipped, because go.sum then
// holds the replacement, or nothing at all.
func goRequirements(dir string) (nameSet, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, err
	}
	// Parse, not ParseLax: ParseLax drops replace directives.
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, fmt.Errorf("go.mod: %w", err)
	}
	replaced := map[string]bool{}
	for _, r := range f.Replace {
		replaced[r.Old.Path] = true
	}
	s := nameSet{}
	for _, r := range f.Require {
		if replaced[r.Mod.Path] {
			continue
		}
		s[r.Mod.Path+" "+r.Mod.Version] = true
	}
	return s, nil
}

// goSumEntries returns "path version" for every module version go.sum lists
// ("v1.2.3/go.mod" lines count as v1.2.3).
func goSumEntries(dir string) (nameSet, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	if err != nil {
		return nil, err
	}
	s := nameSet{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		s[fields[0]+" "+strings.TrimSuffix(fields[1], "/go.mod")] = true
	}
	return s, nil
}

// isPoetryProject reports whether pyproject.toml has a [tool.poetry] table.
// A PEP 621 project (uv, pdm, hatch, setuptools…) needs no poetry.lock.
func isPoetryProject(dir string) bool {
	var m struct {
		Tool map[string]interface{} `toml:"tool"`
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "pyproject.toml"), &m); err != nil {
		return false
	}
	_, ok := m.Tool["poetry"]
	return ok
}

// parsesAsJSON reports a parse error for a JSON lockfile.
func parsesAsJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var v interface{}
	return json.Unmarshal(data, &v)
}

// parsesAsTOML reports a parse error for a TOML lockfile.
func parsesAsTOML(path string) error {
	var v map[string]interface{}
	_, err := toml.DecodeFile(path, &v)
	return err
}

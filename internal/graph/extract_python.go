package graph

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// PythonExtractor extracts dependencies from Python projects.
type PythonExtractor struct{}

func (e *PythonExtractor) Name() string {
	return "python"
}

func (e *PythonExtractor) Supports(file string) bool {
	return file == "requirements.txt" || file == "pyproject.toml" || file == "poetry.lock"
}

// Extract uses poetry.lock (with pyproject.toml for the roots) when present;
// otherwise pyproject.toml's declared dependencies, then requirements.txt, as
// flat roots. A file that exists but does not parse is an error, never a
// silent fallback.
func (e *PythonExtractor) Extract(dir string, _ ExtractOptions) (*DepGraph, error) {
	fallback := dirName(dir)
	pyproject, err := readOptional(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		return nil, err
	}
	lock, err := os.ReadFile(filepath.Join(dir, "poetry.lock"))
	switch {
	case err == nil:
		return parsePoetryLock(lock, pyproject, fallback)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	reqs, err := readOptional(filepath.Join(dir, "requirements.txt"))
	if err != nil {
		return nil, err
	}
	if pyproject != nil {
		info, err := parsePyproject(pyproject)
		if err != nil {
			return nil, err
		}
		if len(info.Deps) > 0 || reqs == nil {
			return flatPythonGraph(info, fallback), nil
		}
	}
	if reqs == nil {
		return nil, fmt.Errorf("no poetry.lock, pyproject.toml or requirements.txt found")
	}
	return parseRequirements(reqs, fallback)
}

var pep503Sep = regexp.MustCompile(`[-_.]+`)

// pep503 normalizes a distribution name ("Typing_Extensions" -> "typing-extensions").
func pep503(name string) string {
	return pep503Sep.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
}

// pep508Name matches the distribution name at the start of a PEP 508 requirement.
var pep508Name = regexp.MustCompile(`^\s*([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)`)

type pyprojectFile struct {
	Project struct {
		Name                 string              `toml:"name"`
		Version              string              `toml:"version"`
		Dependencies         []string            `toml:"dependencies"`
		OptionalDependencies map[string][]string `toml:"optional-dependencies"`
	} `toml:"project"`
	DependencyGroups map[string][]interface{} `toml:"dependency-groups"`
	Tool             struct {
		Poetry struct {
			Name            string                 `toml:"name"`
			Version         string                 `toml:"version"`
			Dependencies    map[string]interface{} `toml:"dependencies"`
			DevDependencies map[string]interface{} `toml:"dev-dependencies"`
			Group           map[string]struct {
				Dependencies map[string]interface{} `toml:"dependencies"`
			} `toml:"group"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

// pyprojectInfo is what the graph needs from pyproject.toml.
type pyprojectInfo struct {
	Name, Version string
	Deps          []string // declared direct dependency names, "python" excluded
}

// parsePyproject reads the project name/version ([project], else
// [tool.poetry]) and the declared direct dependencies: [project]
// dependencies and optional-dependencies, PEP 735 dependency-groups, and
// [tool.poetry] dependencies, dev-dependencies and groups.
func parsePyproject(data []byte) (pyprojectInfo, error) {
	var p pyprojectFile
	if _, err := toml.Decode(string(data), &p); err != nil {
		return pyprojectInfo{}, fmt.Errorf("pyproject.toml: %w", err)
	}
	info := pyprojectInfo{Name: p.Project.Name, Version: p.Project.Version}
	if info.Name == "" {
		info.Name, info.Version = p.Tool.Poetry.Name, p.Tool.Poetry.Version
	}
	seen := map[string]bool{}
	add := func(name string) {
		key := pep503(name)
		if key == "" || key == "python" || seen[key] {
			return
		}
		seen[key] = true
		info.Deps = append(info.Deps, strings.TrimSpace(name))
	}
	addReq := func(req string) {
		if m := pep508Name.FindStringSubmatch(req); m != nil {
			add(m[1])
		}
	}
	for _, r := range p.Project.Dependencies {
		addReq(r)
	}
	for _, extra := range sortedKeys(p.Project.OptionalDependencies) {
		for _, r := range p.Project.OptionalDependencies[extra] {
			addReq(r)
		}
	}
	for _, grp := range sortedKeys(p.DependencyGroups) {
		for _, r := range p.DependencyGroups[grp] {
			if s, ok := r.(string); ok { // tables are {include-group = "..."}
				addReq(s)
			}
		}
	}
	poetry := p.Tool.Poetry
	for _, n := range sortedKeys(poetry.Dependencies) {
		add(n)
	}
	for _, n := range sortedKeys(poetry.DevDependencies) {
		add(n)
	}
	for _, grp := range sortedKeys(poetry.Group) {
		for _, n := range sortedKeys(poetry.Group[grp].Dependencies) {
			add(n)
		}
	}
	return info, nil
}

// parsePoetryLock builds the graph of a poetry.lock. Dependency names are
// matched after PEP 503 normalization. The root is the project from
// pyproject.toml (else fallback), with edges to its declared dependencies.
func parsePoetryLock(data, pyproject []byte, fallback string) (*DepGraph, error) {
	var lock struct {
		Package []struct {
			Name         string                 `toml:"name"`
			Version      string                 `toml:"version"`
			Dependencies map[string]interface{} `toml:"dependencies"`
		} `toml:"package"`
	}
	if _, err := toml.Decode(string(data), &lock); err != nil {
		return nil, fmt.Errorf("poetry.lock: %w", err)
	}
	g := NewGraph()
	byName := map[string]string{} // normalized name -> node ID
	for _, p := range lock.Package {
		if p.Name == "" || p.Version == "" {
			return nil, fmt.Errorf("poetry.lock: [[package]] without name or version")
		}
		n := NewDepNode("python", p.Name, p.Version)
		g.AddNode(n)
		byName[pep503(p.Name)] = n.ID
	}
	for _, p := range lock.Package {
		from := NodeID("python", p.Name, p.Version)
		for _, dep := range sortedKeys(p.Dependencies) {
			if to, ok := byName[pep503(dep)]; ok {
				g.AddEdge(NewEdge(from, to))
			}
		}
	}
	var info pyprojectInfo
	if pyproject != nil {
		var err error
		if info, err = parsePyproject(pyproject); err != nil {
			return nil, err
		}
	}
	project := addProject(g, "python", info.Name, info.Version, fallback)
	for _, n := range info.Deps {
		if id, ok := byName[pep503(n)]; ok {
			g.AddEdge(NewEdge(project, id))
		}
	}
	return g, nil
}

// flatPythonGraph is the project with an edge to one versionless node per
// declared dependency (no lockfile, so nothing is resolved).
func flatPythonGraph(info pyprojectInfo, fallback string) *DepGraph {
	g := NewGraph()
	project := addProject(g, "python", info.Name, info.Version, fallback)
	for _, n := range info.Deps {
		node := NewDepNode("python", n, "")
		g.AddNode(node)
		g.AddEdge(NewEdge(project, node.ID))
	}
	return g
}

// parseRequirements reads requirements.txt as the project (named fallback)
// with an edge to each requirement. Exact pins ("name==1.2.3") keep their
// version; other lines are versionless. Options (-r, -e, --index-url) and
// URLs are skipped; a line that is not a requirement (for example a
// merge-conflict marker) is an error.
func parseRequirements(data []byte, fallback string) (*DepGraph, error) {
	g := NewGraph()
	project := addProject(g, "python", "", "", fallback)
	sc := bufio.NewScanner(bytes.NewReader(data))
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		if i := strings.Index(line, " #"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "\\"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") || strings.Contains(line, "://") {
			continue
		}
		m := pep508Name.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("requirements.txt line %d: not a requirement: %q", lineNo, line)
		}
		version := ""
		rest := strings.TrimSpace(line[len(m[0]):])
		if strings.HasPrefix(rest, "[") { // extras
			if end := strings.IndexByte(rest, ']'); end >= 0 {
				rest = strings.TrimSpace(rest[end+1:])
			}
		}
		if spec, _, _ := strings.Cut(rest, ";"); strings.HasPrefix(strings.TrimSpace(spec), "==") && !strings.Contains(spec, ",") {
			version = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(spec), "=="))
		}
		n := NewDepNode("python", m[1], version)
		g.AddNode(n)
		g.AddEdge(NewEdge(project, n.ID))
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("requirements.txt: %w", err)
	}
	return g, nil
}

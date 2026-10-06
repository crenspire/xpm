package graph

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// JavaExtractor extracts dependencies from Maven and Gradle projects.
type JavaExtractor struct{}

func (e *JavaExtractor) Name() string {
	return "java"
}

func (e *JavaExtractor) Supports(file string) bool {
	return file == "pom.xml" || file == "gradle.lockfile" ||
		file == "build.gradle" || file == "build.gradle.kts" ||
		file == "settings.gradle" || file == "settings.gradle.kts"
}

// Extract reads Maven (pom.xml) first, else Gradle. Without opts.Exec it
// only parses files: pom.xml's direct dependencies, or gradle.lockfile.
// With opts.Exec it runs `mvn dependency:tree` / `gradle dependencies` for
// the full tree and falls back to the files, with a warning, on failure.
func (e *JavaExtractor) Extract(dir string, opts ExtractOptions) (*DepGraph, error) {
	name := dirName(dir)
	if pom, err := os.ReadFile(filepath.Join(dir, "pom.xml")); err == nil {
		if opts.Exec {
			g, err := mavenTree(dir, opts)
			if err == nil {
				return g, nil
			}
			opts.warn("java: `mvn dependency:tree` failed, using pom.xml direct dependencies only: %v", err)
		}
		return parsePom(pom, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	if opts.Exec {
		out, err := opts.run(dir, wrapperOr(dir, "gradlew", "gradlew.bat", "gradle"),
			"dependencies", "--configuration", "runtimeClasspath", "--console=plain")
		if err == nil {
			g, perr := parseGradleDependencies(out, name)
			if perr == nil {
				return g, nil
			}
			err = perr
		}
		opts.warn("java: `gradle dependencies` failed, using gradle.lockfile: %v", err)
	}
	lock, err := os.ReadFile(filepath.Join(dir, "gradle.lockfile"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no gradle.lockfile (enable Gradle dependency locking, or pass --exec to run gradle)")
	}
	if err != nil {
		return nil, err
	}
	return parseGradleLockfile(lock, name)
}

// wrapperOr returns the project's wrapper script (gradlew/mvnw, .bat/.cmd on
// Windows) as an absolute path when it exists, else the plain tool name.
func wrapperOr(dir, unix, windows, tool string) string {
	name := unix
	if runtime.GOOS == "windows" {
		name = windows
	}
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	return tool
}

// mavenTree runs `mvn dependency:tree` in TGF format into a temp file
// (-q keeps stdout quiet; appendOutput collects every reactor module).
func mavenTree(dir string, opts ExtractOptions) (*DepGraph, error) {
	f, err := os.CreateTemp("", "xpm-mvn-*.tgf")
	if err != nil {
		return nil, err
	}
	out := f.Name()
	defer os.Remove(out)
	if err := f.Close(); err != nil {
		return nil, err
	}
	if _, err := opts.run(dir, wrapperOr(dir, "mvnw", "mvnw.cmd", "mvn"), "-q", "dependency:tree",
		"-DoutputType=tgf", "-DoutputFile="+out, "-DappendOutput=true"); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	return parseMavenTGF(data)
}

type pomDependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Optional   string `xml:"optional"`
}

type pomProject struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Parent     struct {
		GroupID string `xml:"groupId"`
		Version string `xml:"version"`
	} `xml:"parent"`
	Properties struct {
		Entries []struct {
			XMLName xml.Name
			Value   string `xml:",chardata"`
		} `xml:",any"`
	} `xml:"properties"`
	// Only <project><dependencies>: dependencyManagement, profiles and
	// plugin dependencies sit at other paths and are not matched.
	Dependencies []pomDependency `xml:"dependencies>dependency"`
}

// xmlCharsetReader accepts the encodings POMs declare: UTF-8/ASCII pass
// through, ISO-8859-1 (Latin-1) is converted to UTF-8.
func xmlCharsetReader(charset string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return in, nil
	case "iso-8859-1", "iso8859-1", "latin1", "latin-1":
		data, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		runes := make([]rune, len(data))
		for i, b := range data {
			runes[i] = rune(b)
		}
		return strings.NewReader(string(runes)), nil
	}
	return nil, fmt.Errorf("unsupported XML encoding %q", charset)
}

var pomProperty = regexp.MustCompile(`\$\{([^}]+)\}`)

// parsePom builds the project (groupId:artifactId@version, inheriting from
// <parent>) with an edge to each direct <dependency>. ${...} references are
// resolved from <properties> and project.*; unknown ones stay as written,
// and a missing version (managed by a parent or BOM) stays empty.
func parsePom(data []byte, fallback string) (*DepGraph, error) {
	var p pomProject
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = xmlCharsetReader
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("pom.xml: %w", err)
	}
	if p.GroupID == "" {
		p.GroupID = p.Parent.GroupID
	}
	if p.Version == "" {
		p.Version = p.Parent.Version
	}
	props := map[string]string{
		"project.groupId": p.GroupID, "project.artifactId": p.ArtifactID, "project.version": p.Version,
		"pom.groupId": p.GroupID, "pom.artifactId": p.ArtifactID, "pom.version": p.Version,
		"project.parent.version": p.Parent.Version,
	}
	for _, e := range p.Properties.Entries {
		props[e.XMLName.Local] = strings.TrimSpace(e.Value)
	}
	expand := func(s string) string {
		return pomProperty.ReplaceAllStringFunc(strings.TrimSpace(s), func(ref string) string {
			if v, ok := props[ref[2:len(ref)-1]]; ok && !strings.Contains(v, "${") {
				return v
			}
			return ref
		})
	}
	g := NewGraph()
	name := ""
	if p.ArtifactID != "" {
		name = expand(p.GroupID) + ":" + expand(p.ArtifactID)
	}
	project := addProject(g, "java", name, expand(p.Version), fallback)
	for _, d := range p.Dependencies {
		if d.GroupID == "" || d.ArtifactID == "" {
			return nil, fmt.Errorf("pom.xml: <dependency> without groupId or artifactId")
		}
		n := NewDepNode("java", expand(d.GroupID)+":"+expand(d.ArtifactID), expand(d.Version))
		scope := strings.TrimSpace(d.Scope)
		if scope == "" {
			scope = "compile"
		}
		n.WithMetadata("scope", scope)
		if strings.TrimSpace(d.Optional) == "true" {
			n.WithMetadata("optional", "true")
		}
		if g.GetNode(n.ID) == nil {
			g.AddNode(n)
		}
		g.AddEdge(NewEdge(project, n.ID))
	}
	return g, nil
}

// mavenLabel parses a TGF node label: groupId:artifactId:type:version[:scope]
// or groupId:artifactId:type:classifier:version:scope.
func mavenLabel(label string) (name, version, scope string, ok bool) {
	p := strings.Split(label, ":")
	switch len(p) {
	case 4:
		return p[0] + ":" + p[1], p[3], "", true
	case 5:
		return p[0] + ":" + p[1], p[3], p[4], true
	case 6:
		return p[0] + ":" + p[1], p[4], p[5], true
	}
	return "", "", "", false
}

// parseMavenTGF parses `mvn dependency:tree -DoutputType=tgf` output: blocks
// of "id label" node lines, a "#" line, then "from to scope" edge lines. With
// -DappendOutput a multi-module build appends one block per module; the
// first node of each block (the module) is a root.
func parseMavenTGF(data []byte) (*DepGraph, error) {
	g := NewGraph()
	ids := map[string]string{}
	inEdges, first := false, true
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "#" {
			inEdges = true
			continue
		}
		f := strings.Fields(line)
		if inEdges && len(f) == 2 && strings.Contains(f[1], ":") { // next module's block
			inEdges, first, ids = false, true, map[string]string{}
		}
		if !inEdges {
			if len(f) != 2 {
				return nil, fmt.Errorf("tgf line %d: want \"id label\", got %q", i+1, line)
			}
			name, version, scope, ok := mavenLabel(f[1])
			if !ok {
				return nil, fmt.Errorf("tgf line %d: cannot parse %q", i+1, f[1])
			}
			n := NewDepNode("java", name, version)
			if scope != "" {
				n.WithMetadata("scope", scope)
			}
			if g.GetNode(n.ID) == nil {
				g.AddNode(n)
			}
			ids[f[0]] = n.ID
			if first {
				g.AddRoot(n.ID)
				first = false
			}
			continue
		}
		if len(f) < 2 {
			return nil, fmt.Errorf("tgf line %d: want \"from to [scope]\", got %q", i+1, line)
		}
		from, ok1 := ids[f[0]]
		to, ok2 := ids[f[1]]
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("tgf line %d: edge references an unknown node", i+1)
		}
		g.AddEdge(NewEdge(from, to))
	}
	if len(g.Root) == 0 {
		return nil, fmt.Errorf("tgf: no nodes")
	}
	return g, nil
}

// parseGradleLockfile reads gradle.lockfile ("group:artifact:version=conf,..."
// lines; "empty=..." lists configurations with no dependencies) as the
// project (named fallback) with an edge to each locked module.
func parseGradleLockfile(data []byte, fallback string) (*DepGraph, error) {
	g := NewGraph()
	project := addProject(g, "java", "", "", fallback)
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		coords, confs, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("gradle.lockfile line %d: missing '='", i+1)
		}
		if coords == "empty" {
			continue
		}
		p := strings.Split(coords, ":")
		if len(p) != 3 || p[0] == "" || p[1] == "" || p[2] == "" {
			return nil, fmt.Errorf("gradle.lockfile line %d: want group:artifact:version, got %q", i+1, coords)
		}
		n := NewDepNode("java", p[0]+":"+p[1], p[2])
		n.WithMetadata("configurations", confs)
		g.AddNode(n)
		g.AddEdge(NewEdge(project, n.ID))
	}
	return g, nil
}

var gradleRootProject = regexp.MustCompile(`^Root project '([^']+)'`)

// gradleNode parses one tree entry ("g:a:1.0", "g:a:1.0 -> 2.0", "g:a -> 2.0",
// "project :core", with optional "(*)", "(n)" or "FAILED" suffixes). ok is
// false for "(c)" constraints, which are not dependencies.
func gradleNode(spec string) (name, version string, ok bool, err error) {
	spec = strings.TrimSpace(spec)
	for _, suffix := range []string{" (*)", " (n)", " FAILED"} {
		spec = strings.TrimSuffix(spec, suffix)
	}
	if strings.HasSuffix(spec, " (c)") {
		return "", "", false, nil
	}
	if rest, isProject := strings.CutPrefix(spec, "project "); isProject {
		return rest, "", true, nil
	}
	coords, target, arrow := strings.Cut(spec, " -> ")
	p := strings.Split(coords, ":")
	if len(p) < 2 || p[0] == "" || p[1] == "" {
		return "", "", false, fmt.Errorf("cannot parse dependency %q", spec)
	}
	if len(p) >= 3 {
		version = p[2]
	}
	if arrow {
		version = strings.TrimSpace(target)
	}
	return p[0] + ":" + p[1], version, true, nil
}

// parseGradleDependencies parses the first configuration tree of
// `gradle dependencies --console=plain` output. The root is the project
// ("Root project 'name'", else fallback); repeated subtrees "(*)" are
// linked, not expanded again.
func parseGradleDependencies(out []byte, fallback string) (*DepGraph, error) {
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	name := ""
	for _, l := range lines {
		if m := gradleRootProject.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			name = m[1]
			break
		}
	}
	g := NewGraph()
	project := addProject(g, "java", name, "", fallback)
	var stack []string // stack[d] is the node at depth d; "" for a constraint
	seen := false
	for i, line := range lines {
		at := strings.Index(line, "+--- ")
		if at < 0 {
			at = strings.Index(line, `\--- `)
		}
		if at < 0 || at%5 != 0 || strings.Trim(line[:at], " |") != "" {
			if seen && strings.TrimSpace(line) == "" {
				break // end of the first configuration
			}
			continue
		}
		seen = true
		depth := at / 5
		if depth > len(stack) {
			return nil, fmt.Errorf("gradle output line %d: indentation skips a level", i+1)
		}
		stack = stack[:depth]
		depName, version, ok, err := gradleNode(line[at+5:])
		if err != nil {
			return nil, fmt.Errorf("gradle output line %d: %w", i+1, err)
		}
		if !ok {
			stack = append(stack, "")
			continue
		}
		n := NewDepNode("java", depName, version)
		if g.GetNode(n.ID) == nil {
			g.AddNode(n)
		}
		parent := project
		if depth > 0 {
			parent = stack[depth-1]
		}
		if parent != "" {
			g.AddEdge(NewEdge(parent, n.ID))
		}
		stack = append(stack, n.ID)
	}
	if !seen && !bytes.Contains(out, []byte("No dependencies")) {
		return nil, fmt.Errorf("no dependency tree in gradle output")
	}
	return g, nil
}

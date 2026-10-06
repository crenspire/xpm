package workspace

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// maxWalkDepth bounds every directory walk and every "**" expansion, counted
// in path segments below the workspace root.
const maxWalkDepth = 6

// skippedDirs are never descended into by a walk, a "*" or a "**": they hold
// installed dependencies, virtualenvs, build output or test fixtures, and may
// contain manifests that are not workspace projects. A pattern that names one
// literally (e.g. "vendor/acme") still reaches it.
var skippedDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"venv":         true,
	"target":       true,
	"testdata":     true,
	"__pycache__":  true,
}

// skipDir reports whether a walk must not enter a directory with this name.
// Hidden directories (.git, .venv, .idea, ...) are always skipped.
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || skippedDirs[name]
}

// walkDirs visits every directory below root (not root itself), skipping
// skipDir names and anything deeper than maxWalkDepth. When visit returns
// true the directory is a project and its subtree is not entered.
func walkDirs(root string, visit func(dir string) bool) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == root {
			return nil
		}
		if skipDir(d.Name()) {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil || len(strings.Split(filepath.ToSlash(rel), "/")) > maxWalkDepth {
			return filepath.SkipDir
		}
		if visit(p) {
			return filepath.SkipDir
		}
		return nil
	})
}

// cleanPattern normalises a workspace glob: slash-separated, no leading "./",
// no trailing "/".
func cleanPattern(p string) string {
	p = filepath.ToSlash(strings.TrimSpace(p))
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	return strings.TrimSuffix(p, "/")
}

// matchPath reports whether the slash-separated relative path rel matches
// pattern: path.Match per segment, plus "**" matching zero or more segments.
func matchPath(pattern, rel string) bool {
	pattern, rel = cleanPattern(pattern), cleanPattern(rel)
	if rel == "" || rel == "." {
		// The workspace root matches only "." or a pattern of "**"s.
		return pattern == "." || strings.Trim(strings.ReplaceAll(pattern, "**", ""), "/") == ""
	}
	if pattern == "" || pattern == "." {
		return false
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			if len(rest) == 0 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchSegments(rest, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// expandGlobs returns the directories under root matched by patterns, as
// absolute paths sorted by their relative slash path. Patterns are relative
// to root and slash-separated; "*", "?" and "[...]" match within one segment,
// "**" matches any number of segments (up to maxWalkDepth). A pattern starting
// with "!" removes the directories it matches from the result, whatever its
// position in the list (npm/pnpm/cargo semantics for the common cases).
// Wildcards never enter skipDir directories; root itself is never returned.
func expandGlobs(root string, patterns []string) []string {
	var include, exclude []string
	for _, p := range patterns {
		if strings.HasPrefix(p, "!") {
			exclude = append(exclude, cleanPattern(p[1:]))
		} else if c := cleanPattern(p); c != "" && c != "." {
			include = append(include, c)
		}
	}
	found := map[string]bool{}
	for _, p := range include {
		expandSegments(root, strings.Split(p, "/"), "", 0, found)
	}
	var rels []string
	for rel := range found {
		if rel == "" || excluded(rel, exclude) {
			continue
		}
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	out := make([]string, 0, len(rels))
	for _, rel := range rels {
		out = append(out, filepath.Join(root, filepath.FromSlash(rel)))
	}
	return out
}

// excluded reports whether rel matches any exclusion pattern, or lies inside
// a directory that one names (Cargo's exclude is a list of paths).
func excluded(rel string, patterns []string) bool {
	for _, p := range patterns {
		if matchPath(p, rel) || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

func expandSegments(root string, segs []string, rel string, depth int, found map[string]bool) {
	if len(segs) == 0 {
		if isDir(filepath.Join(root, filepath.FromSlash(rel))) {
			found[rel] = true
		}
		return
	}
	seg := segs[0]
	switch {
	case seg == "**":
		expandSegments(root, segs[1:], rel, depth, found)
		if depth >= maxWalkDepth {
			return
		}
		for _, name := range subdirs(root, rel) {
			expandSegments(root, segs, joinRel(rel, name), depth+1, found)
		}
	case strings.ContainsAny(seg, "*?["):
		for _, name := range subdirs(root, rel) {
			if ok, err := path.Match(seg, name); err == nil && ok {
				expandSegments(root, segs[1:], joinRel(rel, name), depth+1, found)
			}
		}
	default:
		expandSegments(root, segs[1:], joinRel(rel, seg), depth+1, found)
	}
}

// subdirs lists the directories in root/rel that wildcards may enter.
func subdirs(root, rel string) []string {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !skipDir(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

func joinRel(rel, name string) string {
	if rel == "" {
		return name
	}
	return path.Clean(rel + "/" + name)
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// relSlash returns p relative to root, slash-separated; "." for root itself.
func relSlash(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

package lock

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// errBinaryLockfile is returned by countPackages for formats it cannot read.
var errBinaryLockfile = errors.New("binary lockfile; package count recorded as 0 (bun >= 1.2 writes a text bun.lock)")

// ParseLockfile reads a detected lockfile once and returns its metadata. A
// read error is returned as err. A file whose packages cannot be counted still
// yields a LockInfo (Packages = 0) plus a warning saying why.
func ParseLockfile(d DetectedLockfile) (info *LockInfo, warning string, err error) {
	data, err := os.ReadFile(d.Path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", d.RelPath, err)
	}
	n, countErr := countPackages(d.Spec.File, data)
	if countErr != nil {
		warning = fmt.Sprintf("%s: %v", d.RelPath, countErr)
	}
	return &LockInfo{
		Ecosystem: d.Spec.Ecosystem,
		Manager:   d.Spec.Manager,
		File:      d.RelPath,
		Hash:      hashBytes(data),
		Packages:  n,
	}, warning, nil
}

// ComputeHash returns the lowercase hex SHA-256 of the file at path.
func ComputeHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// countPackages counts the packages a lockfile resolves. It never panics on
// malformed input; it returns 0 and an error instead.
func countPackages(file string, data []byte) (int, error) {
	switch file {
	case "package-lock.json":
		return countPackageLockJSON(data)
	case "yarn.lock":
		return countYarnLock(data), nil
	case "pnpm-lock.yaml":
		return countPnpmLock(data)
	case "bun.lock":
		return countBunLock(data)
	case "bun.lockb":
		return 0, errBinaryLockfile
	case "composer.lock":
		return countComposerLock(data)
	case "poetry.lock":
		return countTOMLPackages(data, false)
	case "uv.lock":
		return countTOMLPackages(data, true)
	case "requirements.lock":
		return countRequirementsLock(data), nil
	case "Pipfile.lock":
		return countPipfileLock(data)
	case "Cargo.lock":
		return countTOMLPackages(data, false)
	case "go.sum":
		return countGoSum(data), nil
	case "gradle.lockfile":
		return countGradleLock(data), nil
	default:
		return 0, fmt.Errorf("unsupported lockfile %q", file)
	}
}

// countPackageLockJSON counts installed packages: "packages" minus the root
// entry "" (lockfileVersion 2/3), or top-level "dependencies" (version 1).
func countPackageLockJSON(data []byte) (int, error) {
	var lf struct {
		Packages     map[string]json.RawMessage `json:"packages"`
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &lf); err != nil {
		return 0, fmt.Errorf("parse package-lock.json: %w", err)
	}
	if lf.Packages != nil {
		n := len(lf.Packages)
		if _, ok := lf.Packages[""]; ok {
			n--
		}
		return n, nil
	}
	return len(lf.Dependencies), nil
}

// countYarnLock counts entry headers: unindented, non-comment lines ending in
// ":" (one per resolution, even when several ranges share it). The Berry
// "__metadata:" header is not a package.
func countYarnLock(data []byte) int {
	n := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t\r")
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		if !strings.HasSuffix(line, ":") || strings.HasPrefix(line, "__metadata") {
			continue
		}
		n++
	}
	return n
}

// countPnpmLock counts the keys of the top-level "packages" map (v5/v6 keys
// look like "/name@1.0.0", v9 keys like "name@1.0.0").
func countPnpmLock(data []byte) (int, error) {
	var lf struct {
		Packages map[string]yaml.Node `yaml:"packages"`
	}
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return 0, fmt.Errorf("parse pnpm-lock.yaml: %w", err)
	}
	return len(lf.Packages), nil
}

// countBunLock counts the keys of "packages" in a text bun.lock (JSON with
// trailing commas).
func countBunLock(data []byte) (int, error) {
	var lf struct {
		Packages map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(stripJSONC(data), &lf); err != nil {
		return 0, fmt.Errorf("parse bun.lock: %w", err)
	}
	return len(lf.Packages), nil
}

// stripJSONC turns JSONC into JSON: it removes // and /* */ comments and
// commas that directly precede "}" or "]", leaving string contents intact.
// Malformed input (unterminated strings or comments) is passed through as far
// as it goes; json.Unmarshal then reports the error.
func stripJSONC(in []byte) []byte {
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); i++ {
		c := in[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(in) && in[j] != '"' {
				if in[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(in) {
				return append(out, in[i:]...)
			}
			out = append(out, in[i:j+1]...)
			i = j
		case c == '/' && i+1 < len(in) && in[i+1] == '/':
			for i < len(in) && in[i] != '\n' {
				i++
			}
			if i < len(in) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(in) && in[i+1] == '*':
			end := bytes.Index(in[i+2:], []byte("*/"))
			if end < 0 {
				return out
			}
			i += 2 + end + 1
		case c == ',':
			j := i + 1
			for j < len(in) && (in[j] == ' ' || in[j] == '\t' || in[j] == '\n' || in[j] == '\r') {
				j++
			}
			if j < len(in) && (in[j] == '}' || in[j] == ']') {
				continue
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

// countComposerLock counts "packages" plus "packages-dev".
func countComposerLock(data []byte) (int, error) {
	var lf struct {
		Packages    []json.RawMessage `json:"packages"`
		PackagesDev []json.RawMessage `json:"packages-dev"`
	}
	if err := json.Unmarshal(data, &lf); err != nil {
		return 0, fmt.Errorf("parse composer.lock: %w", err)
	}
	return len(lf.Packages) + len(lf.PackagesDev), nil
}

// countTOMLPackages counts [[package]] tables (poetry.lock, uv.lock,
// Cargo.lock). With skipRoot, uv's entry for the project itself
// (source = { editable = "." } or { virtual = "." }) is not counted.
func countTOMLPackages(data []byte, skipRoot bool) (int, error) {
	var lf struct {
		Package []struct {
			Source map[string]interface{} `toml:"source"`
		} `toml:"package"`
	}
	if _, err := toml.Decode(string(data), &lf); err != nil {
		return 0, fmt.Errorf("parse TOML lockfile: %w", err)
	}
	n := 0
	for _, p := range lf.Package {
		if skipRoot && (p.Source["editable"] == "." || p.Source["virtual"] == ".") {
			continue
		}
		n++
	}
	return n, nil
}

// countRequirementsLock counts requirement lines (not blank, comments, or
// pip options such as "-r" / "--hash" continuation lines).
func countRequirementsLock(data []byte) int {
	n := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "-") {
			n++
		}
	}
	return n
}

// countPipfileLock counts "default" plus "develop" entries.
func countPipfileLock(data []byte) (int, error) {
	var lf struct {
		Default map[string]json.RawMessage `json:"default"`
		Develop map[string]json.RawMessage `json:"develop"`
	}
	if err := json.Unmarshal(data, &lf); err != nil {
		return 0, fmt.Errorf("parse Pipfile.lock: %w", err)
	}
	return len(lf.Default) + len(lf.Develop), nil
}

// countGoSum counts distinct module@version pairs ("v1.0.0" and
// "v1.0.0/go.mod" lines are the same module version).
func countGoSum(data []byte) int {
	seen := make(map[string]bool)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) < 2 || strings.HasPrefix(parts[0], "//") {
			continue
		}
		seen[parts[0]+"@"+strings.TrimSuffix(parts[1], "/go.mod")] = true
	}
	return len(seen)
}

// countGradleLock counts dependency lines (not blank, comments, or the
// trailing "empty=" line).
func countGradleLock(data []byte) int {
	n := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "empty=") {
			n++
		}
	}
	return n
}

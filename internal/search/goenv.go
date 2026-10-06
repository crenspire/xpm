package search

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
)

// goEnvPath returns the go env file that `go env -w` writes: $GOENV if set,
// none if GOENV=off, else <user config dir>/go/env. Tests point GOENV at a
// temp file so the real user file is never read.
func goEnvPath() string {
	if p := os.Getenv("GOENV"); p != "" {
		if p == "off" {
			return ""
		}
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "go", "env")
}

// goEnv returns the effective value of each key the way the go command sees
// it: the process environment when non-empty, else the go env file. The file
// is read at most once per call; the go command itself is never run.
func goEnv(keys ...string) map[string]string {
	out := make(map[string]string, len(keys))
	var missing []string
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			out[k] = v
		} else {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return out
	}
	path := goEnvPath()
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	file := parseGoEnvFile(data)
	for _, k := range missing {
		out[k] = file[k]
	}
	return out
}

// parseGoEnvFile parses KEY=VALUE lines, skipping blank lines and # comments.
// Values are normally unquoted; a value wrapped in matching quotes is
// unwrapped.
func parseGoEnvFile(data []byte) map[string]string {
	m := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		m[k] = v
	}
	return m
}

// GoModuleIsPrivate reports whether mod matches GOPRIVATE or GONOPROXY, read
// from the environment or, when unset there, from the go env file (`go env
// -w`). Such modules are never sent to a public proxy or vulnerability
// database.
func GoModuleIsPrivate(mod string) bool {
	return goModuleIsPrivate(goEnv("GOPRIVATE", "GONOPROXY"), mod)
}

func goModuleIsPrivate(env map[string]string, mod string) bool {
	return module.MatchPrefixPatterns(env["GOPRIVATE"], mod) || module.MatchPrefixPatterns(env["GONOPROXY"], mod)
}

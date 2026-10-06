package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePnpmLockV9Snapshots(t *testing.T) {
	g, err := parsePnpmLock(fixture(t, "pnpm/v9/pnpm-lock.yaml"), nil, "pnpm-app")
	if err != nil {
		t.Fatal(err)
	}
	// 9 packages + the project (no package.json: named by the fallback).
	wantGraph(t, g, 10, 12, 1, []string{
		"node:react-dom@18.3.1 -> node:react@18.3.1", // snapshot key carries "(react@18.3.1)"
		"node:react-dom@18.3.1 -> node:scheduler@0.23.2",
		"node:@types/react@18.3.3 -> node:@types/prop-types@15.7.12",
		"node:loose-envify@1.4.0 -> node:js-tokens@4.0.0",
		"node:pnpm-app@ -> node:react-dom@18.3.1",
		"node:pnpm-app@ -> node:react-is@16.13.1", // npm: alias "react-is-legacy" resolves to the real package
		"node:pnpm-app@ -> node:@types/react@18.3.3",
	}, []string{"node:pnpm-app@"})
}

func TestParsePnpmLockV6(t *testing.T) {
	g, err := parsePnpmLock(fixture(t, "pnpm/v6/pnpm-lock.yaml"), []byte(`{"name": "pnpm6-app", "version": "1.0.0"}`), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 9, 11, 1, []string{
		"node:react-dom@18.2.0 -> node:react@18.2.0",
		"node:react-dom@18.2.0 -> node:scheduler@0.23.0",
		"node:@types/react@18.2.79 -> node:csstype@3.1.3",
		"node:pnpm6-app@1.0.0 -> node:react-dom@18.2.0", // top-level (pre-importers) section
	}, []string{"node:pnpm6-app@1.0.0"})
}

func TestPnpmSplitKey(t *testing.T) {
	cases := []struct {
		key           string
		v5            bool
		name, version string
	}{
		{"/@babel/core@7.24.7", false, "@babel/core", "7.24.7"},
		{"/react-dom@18.2.0(react@18.2.0)", false, "react-dom", "18.2.0"},
		{"@testing-library/react@16.0.0(@types/react@18.3.3)(react@18.3.1)", false, "@testing-library/react", "16.0.0"},
		{"/@babel/core/7.24.7", true, "@babel/core", "7.24.7"},
		{"/react-dom/18.2.0_react@18.2.0", true, "react-dom", "18.2.0"},
	}
	for _, c := range cases {
		name, version, ok := pnpmSplitKey(c.key, c.v5)
		if !ok || name != c.name || version != c.version {
			t.Errorf("pnpmSplitKey(%q, %v) = %q, %q, %v; want %q, %q", c.key, c.v5, name, version, ok, c.name, c.version)
		}
	}
}

func TestParsePnpmLockV5(t *testing.T) {
	data := []byte(`lockfileVersion: 5.4

specifiers:
  react-dom: ^18.2.0

dependencies:
  react-dom: 18.2.0_react@18.2.0

packages:

  /react-dom/18.2.0_react@18.2.0:
    resolution: {integrity: sha512-6IMTriUmvsjHUjNtEDudZfuDQUoWXVxKHhlEGSk81n4YFS+r/Kl99wXiwlVXtPBtJenozv2P+hxDsw9eA7Xo6g==}
    peerDependencies:
      react: ^18.2.0
    dependencies:
      react: 18.2.0
    dev: false

  /react/18.2.0:
    resolution: {integrity: sha512-/3IjMdb2L9QbBdWiW5e3P2/npwMBaU9mHCSCUzNln0ZCYbcfTsGbTJrU/kGemdH2IWmB2ioZ+zkxtmq6g09fGQ==}
    dev: false
`)
	g, err := parsePnpmLock(data, nil, "v5-app")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 3, 2, 1, []string{
		"node:react-dom@18.2.0 -> node:react@18.2.0",
		"node:v5-app@ -> node:react-dom@18.2.0",
	}, []string{"node:v5-app@"})
}

func TestParsePnpmLockV9Workspace(t *testing.T) {
	g, err := parsePnpmLock(fixture(t, "pnpm/v9-workspace/pnpm-lock.yaml"), fixture(t, "pnpm/v9-workspace/package.json"), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 5, 6, 1, []string{
		"node:ws-root@ -> node:typescript@5.4.5",
		"node:ws-root@ -> node:packages/app@", // importers become nodes named by path
		"node:ws-root@ -> node:packages/lib@",
		"node:packages/app@ -> node:packages/lib@", // link:../lib
		"node:packages/app@ -> node:kleur@4.1.5",
		"node:packages/lib@ -> node:kleur@4.1.5",
	}, []string{"node:ws-root@"})
}

func TestParsePnpmLockMalformed(t *testing.T) {
	for name, data := range map[string]string{
		"not-yaml":        "lockfileVersion: '9.0'\npackages: [unclosed\n",
		"no-version":      "packages:\n  react@18.3.1: {}\n",
		"bad-package-key": "lockfileVersion: '9.0'\npackages:\n  react: {}\n",
		"packages-list":   "lockfileVersion: '9.0'\npackages:\n  - react\n",
	} {
		t.Run(name, func(t *testing.T) {
			if g, err := parsePnpmLock([]byte(data), nil, "fallback"); err == nil {
				t.Fatalf("want error, got %s", dumpGraph(g))
			}
		})
	}
}

func TestParseYarnLockV1(t *testing.T) {
	g, err := parseYarnLock(fixture(t, "yarn/v1/yarn.lock"), fixture(t, "yarn/v1/package.json"), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 12, 13, 1, []string{
		"node:send@0.18.0 -> node:debug@2.6.9", // "debug@2.6.9" key, not the ^4.3.4 one
		"node:debug@2.6.9 -> node:ms@2.0.0",
		"node:debug@4.3.5 -> node:ms@2.1.2",
		"node:@babel/code-frame@7.24.7 -> node:@babel/highlight@7.24.7",
		"node:@babel/highlight@7.24.7 -> node:picocolors@1.0.1", // entry with two specs
		"node:yarn-app@1.0.0 -> node:debug@4.3.5",
		"node:yarn-app@1.0.0 -> node:picocolors@1.0.1", // devDependencies "^1.0.1"
	}, []string{"node:yarn-app@1.0.0"})
}

func TestParseYarnLockBerry(t *testing.T) {
	// The project is the "berry-app@workspace:." entry; no package.json needed.
	g, err := parseYarnLock(fixture(t, "yarn/berry/yarn.lock"), nil, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 12, 13, 1, []string{
		"node:send@0.18.0 -> node:debug@2.6.9",
		"node:debug@4.3.5 -> node:ms@2.1.2",
		"node:@babel/highlight@7.24.7 -> node:js-tokens@4.0.0",
		"node:berry-app@0.0.0-use.local -> node:send@0.18.0",
		"node:berry-app@0.0.0-use.local -> node:picocolors@1.0.1",
	}, []string{"node:berry-app@0.0.0-use.local"})
}

func TestYarnSplitSpec(t *testing.T) {
	cases := [][3]string{
		{"@babel/core@^7.0.0", "@babel/core", "^7.0.0"},
		{"debug@npm:^4.3.4", "debug", "npm:^4.3.4"},
		{"string-width-cjs@npm:string-width@^4.2.0", "string-width-cjs", "npm:string-width@^4.2.0"},
	}
	for _, c := range cases {
		name, rng, ok := yarnSplitSpec(c[0])
		if !ok || name != c[1] || rng != c[2] {
			t.Errorf("yarnSplitSpec(%q) = %q, %q, %v", c[0], name, rng, ok)
		}
	}
	if got := yarnRealName("string-width-cjs", "npm:string-width@^4.2.0"); got != "string-width" {
		t.Errorf("yarnRealName = %q, want string-width", got)
	}
}

func TestParseYarnLockMalformed(t *testing.T) {
	for name, data := range map[string]string{
		"v1-garbage":      "# yarn lockfile v1\n\nthis is not a lockfile\n",
		"v1-no-version":   "# yarn lockfile v1\n\nms@2.1.2:\n  resolved \"https://registry.yarnpkg.com/ms/-/ms-2.1.2.tgz\"\n",
		"v1-orphan-line":  "# yarn lockfile v1\n\n  version \"1.0.0\"\n",
		"v1-conflict":     "# yarn lockfile v1\n\n<<<<<<< HEAD\nms@2.1.2:\n  version \"2.1.2\"\n",
		"berry-bad-yaml":  "__metadata:\n  version: 8\n\"ms@npm:2.1.2\": [\n",
		"berry-no-resolv": "__metadata:\n  version: 8\n\n\"ms@npm:2.1.2\":\n  version: 2.1.2\n",
	} {
		t.Run(name, func(t *testing.T) {
			if g, err := parseYarnLock([]byte(data), nil, "fallback"); err == nil {
				t.Fatalf("want error, got %s", dumpGraph(g))
			}
		})
	}
}

func TestNodeExtractorPicksPnpmThenYarn(t *testing.T) {
	g, err := (&NodeExtractor{}).Extract(copyFixtureDir(t, "pnpm/v9"), ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 10, 12, 1, nil, nil)

	g, err = (&NodeExtractor{}).Extract(copyFixtureDir(t, "yarn/v1"), ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 12, 13, 1, nil, []string{"node:yarn-app@1.0.0"})
}

func TestNodeExtractorBunIsReportedNotParsed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bun.lock"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&NodeExtractor{}).Extract(dir, ExtractOptions{}); err == nil {
		t.Fatal("want a 'not supported' error for bun.lock")
	}
}

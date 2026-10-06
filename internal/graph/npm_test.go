package graph

import "testing"

func TestParseNpmLockV3Hoisted(t *testing.T) {
	g, err := parseNpmLock(fixture(t, "npm/v3-hoisted/package-lock.json"), nil, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	// 12 packages + the project; 7 package edges + 5 from the project.
	wantGraph(t, g, 13, 12, 1, []string{
		"node:send@0.18.0 -> node:debug@2.6.9",                 // nested under send
		"node:send@0.18.0 -> node:ms@2.1.3",                    // nested under send
		"node:send@0.18.0 -> node:mime@1.6.0",                  // hoisted to the root
		"node:debug@2.6.9 -> node:ms@2.0.0",                    // nested two levels deep
		"node:debug@4.3.4 -> node:ms@2.1.2",                    // hoisted
		"node:@babel/highlight@7.24.7 -> node:js-tokens@4.0.0", // same version at two paths: one node
		"node:@babel/highlight@7.24.7 -> node:@babel/helper-validator-identifier@7.24.7",
		"node:hoisted-app@1.0.0 -> node:typescript@5.4.5", // devDependencies
		"node:hoisted-app@1.0.0 -> node:fsevents@2.3.3",   // optionalDependencies
	}, []string{"node:hoisted-app@1.0.0"})
	if n := g.GetNode("node:@babel/highlight@7.24.7"); n == nil || n.Name != "@babel/highlight" {
		t.Errorf("scoped node = %+v, want name @babel/highlight", n)
	}
}

func TestParseNpmLockV2PrefersPackages(t *testing.T) {
	// No package.json: the project and its direct dependencies must come from
	// the v2 "packages" root entry, which lists has-flag as a devDependency.
	g, err := parseNpmLock(fixture(t, "npm/v2/package-lock.json"), nil, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 9, 9, 1, []string{
		"node:@types/node@20.12.7 -> node:undici-types@5.26.5",
		"node:supports-color@7.2.0 -> node:has-flag@4.0.0",
		"node:app2@2.0.0 -> node:has-flag@4.0.0",
	}, []string{"node:app2@2.0.0"})
}

func TestParseNpmLockV1Nested(t *testing.T) {
	g, err := parseNpmLock(fixture(t, "npm/v1-nested/package-lock.json"), fixture(t, "npm/v1-nested/package.json"), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 10, 9, 1, []string{
		"node:send@0.18.0 -> node:debug@2.6.9",
		"node:send@0.18.0 -> node:ms@2.1.3",
		"node:send@0.18.0 -> node:mime@1.6.0",
		"node:debug@2.6.9 -> node:ms@2.0.0",
		"node:debug@4.3.4 -> node:ms@2.1.2",
		"node:@types/debug@4.1.12 -> node:@types/ms@0.7.34",
		"node:legacy-app@1.0.0 -> node:send@0.18.0",
		"node:legacy-app@1.0.0 -> node:@types/debug@4.1.12",
	}, []string{"node:legacy-app@1.0.0"})
}

func TestParseNpmLockV1WithoutManifestHasNoDirectDeps(t *testing.T) {
	// v1 lockfiles do not record the project's direct dependencies.
	g, err := parseNpmLock(fixture(t, "npm/v1-nested/package-lock.json"), nil, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 10, 6, 1, nil, []string{"node:legacy-app@1.0.0"})
}

func TestParseNpmLockV3Workspaces(t *testing.T) {
	g, err := parseNpmLock(fixture(t, "npm/v3-workspaces/package-lock.json"), nil, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 6, 6, 1, []string{
		"node:@mono/cli@0.3.0 -> node:@mono/core@1.2.0", // through the node_modules link
		"node:@mono/cli@0.3.0 -> node:commander@12.0.0", // packages/cli/node_modules
		"node:@mono/core@1.2.0 -> node:kleur@4.1.5",     // root node_modules
		"node:mono@0.0.0 -> node:typescript@5.4.5",
		"node:mono@0.0.0 -> node:@mono/cli@0.3.0", // workspace packages hang off the project
		"node:mono@0.0.0 -> node:@mono/core@1.2.0",
	}, []string{"node:mono@0.0.0"})
}

func TestParseNpmLockMalformed(t *testing.T) {
	for name, data := range map[string]string{
		"truncated":      `{"lockfileVersion": 3, "packages": {"": {`,
		"packages-array": `{"lockfileVersion": 3, "packages": []}`,
		"version-object": `{"lockfileVersion": 3, "packages": {"node_modules/a": {"version": {}}}}`,
		"not-json":       "<<<<<<< HEAD\n",
	} {
		t.Run(name, func(t *testing.T) {
			if g, err := parseNpmLock([]byte(data), nil, "fallback"); err == nil {
				t.Fatalf("want error, got graph %v", g)
			}
		})
	}
	if _, err := parseNpmLock(fixture(t, "npm/v1-nested/package-lock.json"), []byte("{"), "fallback"); err == nil {
		t.Fatal("malformed package.json must be an error")
	}
}

func TestParseNpmLockProjectNameFallsBack(t *testing.T) {
	g, err := parseNpmLock([]byte(`{"lockfileVersion": 3, "packages": {"": {}}}`), nil, "my-dir")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 1, 0, 1, nil, []string{"node:my-dir@"})
}

package graph

import "testing"

func TestParseComposerLock(t *testing.T) {
	g, err := parseComposerLock(fixture(t, "composer/app/composer.lock"), fixture(t, "composer/app/composer.json"), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	// 10 packages + the project; 10 package edges + 3 from the project.
	wantGraph(t, g, 11, 13, 1, []string{
		"php:monolog/monolog@3.6.0 -> php:psr/log@3.0.0",
		"php:symfony/console@v7.1.1 -> php:symfony/string@v7.1.1",
		"php:symfony/service-contracts@v3.5.0 -> php:psr/container@2.0.2",
		"php:symfony/var-dumper@v7.1.1 -> php:symfony/polyfill-mbstring@v1.30.0", // packages-dev
		"php:acme/app@ -> php:symfony/console@v7.1.1",                            // "Symfony/Console": case-insensitive
		"php:acme/app@ -> php:symfony/var-dumper@v7.1.1",                         // require-dev
	}, []string{"php:acme/app@"})
	for id := range g.Nodes {
		if id == "php:php@" || id == "php:ext-json@" {
			t.Errorf("platform requirement %s became a node", id)
		}
	}
}

func TestParseComposerLockMalformed(t *testing.T) {
	for name, data := range map[string]string{
		"truncated":       `{"packages": [{"name": "psr/log", "version": "3.0.0"`,
		"packages-object": `{"packages": {"psr/log": "3.0.0"}}`,
		"no-version":      `{"packages": [{"name": "psr/log"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if g, err := parseComposerLock([]byte(data), nil, "f"); err == nil {
				t.Fatalf("want error, got %s", dumpGraph(g))
			}
		})
	}
	if _, err := parseComposerLock([]byte(`{"packages": []}`), []byte("{"), "f"); err == nil {
		t.Fatal("malformed composer.json must be an error")
	}
}

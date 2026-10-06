package graph

import "testing"

func TestParseCargoLockKeysByNameAndVersion(t *testing.T) {
	g, err := parseCargoLock(fixture(t, "cargo/workspace/Cargo.lock"))
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 11, 15, 2, []string{
		"rust:app@0.1.0 -> rust:bitflags@2.5.0",      // "bitflags 2.5.0"
		"rust:app-core@0.1.0 -> rust:bitflags@1.3.2", // "bitflags 1.3.2"
		"rust:app@0.1.0 -> rust:anyhow@1.0.86",       // bare name, one locked version
		"rust:app-core@0.1.0 -> rust:serde@1.0.203",  // "name version (source)"
		"rust:app@0.1.0 -> rust:app-core@0.1.0",
		"rust:syn@2.0.66 -> rust:unicode-ident@1.0.12",
	}, []string{"rust:app@0.1.0", "rust:app-core@0.1.0"})
}

func TestParseCargoLockMalformed(t *testing.T) {
	for name, data := range map[string]string{
		"not-toml":       "[[package]\nname = \"x\"\n",
		"no-version":     "version = 4\n\n[[package]]\nname = \"x\"\n",
		"ambiguous-dep":  "[[package]]\nname = \"a\"\nversion = \"1.0.0\"\ndependencies = [\"b\"]\n\n[[package]]\nname = \"b\"\nversion = \"1.0.0\"\n\n[[package]]\nname = \"b\"\nversion = \"2.0.0\"\n",
		"missing-dep":    "[[package]]\nname = \"a\"\nversion = \"1.0.0\"\ndependencies = [\"zzz 9.9.9\"]\n",
		"dependencies-s": "[[package]]\nname = \"a\"\nversion = \"1.0.0\"\ndependencies = \"b\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			if g, err := parseCargoLock([]byte(data)); err == nil {
				t.Fatalf("want error, got %s", dumpGraph(g))
			}
		})
	}
}

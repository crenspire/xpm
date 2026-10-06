package pm

import (
	"testing"
)

// TestAllMetas verifies that AllMetas returns all supported package managers.
func TestAllMetas(t *testing.T) {
	metas := AllMetas()

	expectedCount := 12 // npm, yarn, pnpm, bun, pip, poetry, pipenv, composer, cargo, gomod, maven, gradle
	if len(metas) != expectedCount {
		t.Errorf("expected %d package managers, got %d", expectedCount, len(metas))
	}

	// Verify each expected ID is present
	expectedIDs := []ID{Npm, Yarn, Pnpm, Bun, Pip, Poetry, Pipenv, Composer, Cargo, GoMod, Maven, Gradle}
	for _, expectedID := range expectedIDs {
		found := false
		for _, m := range metas {
			if m.ID == expectedID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected package manager %s not found in AllMetas()", expectedID)
		}
	}
}

// TestMetaFor verifies metadata lookup by ID.
func TestMetaFor(t *testing.T) {
	tests := []struct {
		id       ID
		wantName string
		wantOK   bool
	}{
		{Npm, "npm (Node.js)", true},
		{Yarn, "yarn", true},
		{Pnpm, "pnpm", true},
		{Bun, "bun", true},
		{Pip, "pip (Python)", true},
		{Poetry, "poetry (Python)", true},
		{Pipenv, "pipenv (Python)", true},
		{Composer, "composer (PHP)", true},
		{Cargo, "cargo (Rust)", true},
		{GoMod, "go modules (Go)", true},
		{Maven, "maven (Java)", true},
		{Gradle, "gradle (Java)", true},
		{ID("unknown"), "", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.id), func(t *testing.T) {
			meta, ok := MetaFor(tc.id)
			if ok != tc.wantOK {
				t.Errorf("MetaFor(%s) ok = %v, want %v", tc.id, ok, tc.wantOK)
			}
			if ok && meta.Name != tc.wantName {
				t.Errorf("MetaFor(%s) name = %s, want %s", tc.id, meta.Name, tc.wantName)
			}
		})
	}
}

// TestNewAdapter verifies adapter creation for all supported package managers.
func TestNewAdapter(t *testing.T) {
	tests := []struct {
		id      ID
		wantErr bool
	}{
		{Npm, false},
		{Yarn, false},
		{Pnpm, false},
		{Bun, false},
		{Pip, false},
		{Poetry, false},
		{Pipenv, false},
		{Composer, false},
		{Cargo, false},
		{GoMod, false},
		{Maven, false},
		{Gradle, false},
		{ID("unknown"), true},
	}

	for _, tc := range tests {
		t.Run(string(tc.id), func(t *testing.T) {
			adapter, err := NewAdapter(tc.id)
			if (err != nil) != tc.wantErr {
				t.Errorf("NewAdapter(%s) error = %v, wantErr %v", tc.id, err, tc.wantErr)
			}
			if !tc.wantErr && adapter == nil {
				t.Errorf("NewAdapter(%s) returned nil adapter", tc.id)
			}
			if !tc.wantErr && adapter.ID() != tc.id {
				t.Errorf("NewAdapter(%s).ID() = %s, want %s", tc.id, adapter.ID(), tc.id)
			}
		})
	}
}

// TestExists verifies binary existence checking.
func TestExists(t *testing.T) {
	// Test with a binary that should always exist on Unix systems
	if Exists("sh") != true {
		// Skip on Windows where sh might not exist
		t.Log("Skipping sh test - may not exist on Windows")
	}

	// Test with a binary that definitely doesn't exist
	if Exists("this-binary-definitely-does-not-exist-12345") {
		t.Error("Exists() returned true for non-existent binary")
	}
}

// TestInstallHint verifies install hints are provided for all package managers.
func TestInstallHint(t *testing.T) {
	tests := []struct {
		id        ID
		wantEmpty bool
	}{
		{Bun, false},
		{Pnpm, false},
		{Yarn, false},
		{Pip, false},
		{Poetry, false},
		{Pipenv, false},
		{Composer, false},
		{Cargo, false},
		{GoMod, false},
		{Maven, false},
		{Gradle, false},
		{Npm, false},
		{ID("unknown"), true},
	}

	for _, tc := range tests {
		t.Run(string(tc.id), func(t *testing.T) {
			hint := InstallHint(tc.id)
			isEmpty := hint == ""
			if isEmpty != tc.wantEmpty {
				t.Errorf("InstallHint(%s) isEmpty = %v, want %v", tc.id, isEmpty, tc.wantEmpty)
			}
		})
	}
}

// TestWrap verifies command wrapping with error handling.
func TestWrap(t *testing.T) {
	// Test successful command
	err := Wrap("echo", []string{"hello"})
	if err != nil {
		t.Errorf("Wrap(echo hello) failed: %v", err)
	}

	// Test failed command
	err = Wrap("false", []string{})
	if err == nil {
		t.Error("Wrap(false) should have failed")
	}
}

// TestAdapterID verifies that each adapter returns the correct ID.
func TestAdapterID(t *testing.T) {
	adapters := []struct {
		adapter Adapter
		wantID  ID
	}{
		{NpmAdapter{}, Npm},
		{YarnAdapter{}, Yarn},
		{PnpmAdapter{}, Pnpm},
		{BunAdapter{}, Bun},
		{PipAdapter{}, Pip},
		{PoetryAdapter{}, Poetry},
		{PipenvAdapter{}, Pipenv},
		{ComposerAdapter{}, Composer},
		{CargoAdapter{}, Cargo},
		{GoModAdapter{}, GoMod},
		{MavenAdapter{}, Maven},
		{GradleAdapter{}, Gradle},
	}

	for _, tc := range adapters {
		t.Run(string(tc.wantID), func(t *testing.T) {
			if tc.adapter.ID() != tc.wantID {
				t.Errorf("adapter.ID() = %s, want %s", tc.adapter.ID(), tc.wantID)
			}
		})
	}
}

// MockCommandRunner is a test helper that records commands instead of executing them.
type MockCommandRunner struct {
	Commands   []string
	ShouldFail bool
}

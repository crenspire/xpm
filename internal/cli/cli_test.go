package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
)

// TestVersion verifies the version constant.
func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}

	// Version should be in semver format
	if Version[0] < '0' || Version[0] > '9' {
		t.Errorf("Version should start with a number, got %s", Version)
	}
}

// TestFileExists verifies the fileExists helper function.
func TestFileExists(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	// Should not exist yet
	if fileExists(tmpFile) {
		t.Error("fileExists() returned true for non-existent file")
	}

	// Create the file
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Should exist now
	if !fileExists(tmpFile) {
		t.Error("fileExists() returned false for existing file")
	}
}

// TestPreferOrderMap verifies the preference ordering logic.
func TestPreferOrderMap(t *testing.T) {
	tests := []struct {
		name   string
		prefer []string
		checks map[string]int
	}{
		{
			name:   "empty prefer",
			prefer: []string{},
			checks: map[string]int{"npm": 1000, "pip": 1000},
		},
		{
			name:   "npm first",
			prefer: []string{"npm"},
			checks: map[string]int{"npm": 0, "pip": 1000},
		},
		{
			name:   "pip then npm",
			prefer: []string{"pip", "npm"},
			checks: map[string]int{"pip": 0, "npm": 1, "cargo": 1000},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := preferOrderMap(tc.prefer)
			for k, want := range tc.checks {
				if got := m[k]; got != want {
					t.Errorf("preferOrderMap[%s] = %d, want %d", k, got, want)
				}
			}
		})
	}
}

// TestPickPreferredPM verifies preferred package manager selection.
func TestPickPreferredPM(t *testing.T) {
	tests := []struct {
		name       string
		candidates []pm.ID
		prefer     []string
		want       pm.ID
	}{
		{
			name:       "no preference",
			candidates: []pm.ID{pm.Npm, pm.Yarn, pm.Pnpm},
			prefer:     []string{},
			want:       "",
		},
		{
			name:       "prefer npm",
			candidates: []pm.ID{pm.Npm, pm.Yarn, pm.Pnpm},
			prefer:     []string{"npm"},
			want:       pm.Npm,
		},
		{
			name:       "prefer yarn",
			candidates: []pm.ID{pm.Npm, pm.Yarn, pm.Pnpm},
			prefer:     []string{"yarn"},
			want:       pm.Yarn,
		},
		{
			name:       "prefer first match",
			candidates: []pm.ID{pm.Npm, pm.Yarn, pm.Pnpm},
			prefer:     []string{"pip", "yarn", "npm"},
			want:       pm.Yarn,
		},
		{
			name:       "no candidates match",
			candidates: []pm.ID{pm.Npm, pm.Yarn},
			prefer:     []string{"pip", "cargo"},
			want:       "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickPreferredPM(tc.candidates, tc.prefer)
			if got != tc.want {
				t.Errorf("pickPreferredPM() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDetectProjectTargets verifies project type detection.
func TestDetectProjectTargets(t *testing.T) {
	// Save current directory and restore after test
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current dir: %v", err)
	}
	defer os.Chdir(origDir)

	// Create a temp directory with test files
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Test empty directory
	targets := detectProjectTargets()
	if len(targets) != 0 {
		t.Errorf("detectProjectTargets() in empty dir returned %d targets, want 0", len(targets))
	}

	// Create package.json
	if err := os.WriteFile("package.json", []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	targets = detectProjectTargets()
	if len(targets) != 1 {
		t.Errorf("detectProjectTargets() with package.json returned %d targets, want 1", len(targets))
	}
	if targets[0].Kind != "node" {
		t.Errorf("detectProjectTargets() kind = %s, want node", targets[0].Kind)
	}

	// Add requirements.txt
	if err := os.WriteFile("requirements.txt", []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create requirements.txt: %v", err)
	}

	targets = detectProjectTargets()
	if len(targets) != 2 {
		t.Errorf("detectProjectTargets() with both files returned %d targets, want 2", len(targets))
	}
}

// TestProjectTargetStruct verifies projectTarget structure.
func TestProjectTargetStruct(t *testing.T) {
	target := projectTarget{
		Label: "Node (package.json)",
		Kind:  "node",
		PMs:   []pm.ID{pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun},
	}

	if target.Label == "" {
		t.Error("projectTarget.Label should not be empty")
	}

	if target.Kind == "" {
		t.Error("projectTarget.Kind should not be empty")
	}

	if len(target.PMs) == 0 {
		t.Error("projectTarget.PMs should not be empty")
	}
}

// TestUsageDoesNotPanic verifies usage() doesn't panic.
func TestUsageDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("usage() panicked: %v", r)
		}
	}()

	// Redirect stdout to discard output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	usage()
}

// TestRunWithNoArgs verifies Run() returns error with no arguments.
func TestRunWithNoArgs(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Set args to just the program name
	os.Args = []string{"upm"}

	// Redirect stdout/stderr
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	code := Run()
	if code != 1 {
		t.Errorf("Run() with no args should return 1, got %d", code)
	}
}

// TestRunVersion verifies Run() handles version command.
func TestRunVersion(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Test version command
	os.Args = []string{"upm", "version"}

	// Redirect stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	code := Run()
	if code != 0 {
		t.Errorf("Run() with version should return 0, got %d", code)
	}
}

// TestRunHelp verifies Run() handles help command.
func TestRunHelp(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Test help command
	os.Args = []string{"upm", "help"}

	// Redirect stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	code := Run()
	if code != 0 {
		t.Errorf("Run() with help should return 0, got %d", code)
	}
}

// TestRunUnknownCommand verifies Run() handles unknown commands.
func TestRunUnknownCommand(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"upm", "unknown-command"}

	// Redirect stdout/stderr
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout, _ = os.Open(os.DevNull)
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	code := Run()
	if code != 1 {
		t.Errorf("Run() with unknown command should return 1, got %d", code)
	}
}

// TestVerboseFlag verifies verbose flag parsing.
func TestVerboseFlag(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	tests := []string{"-v", "--verbose"}

	for _, flag := range tests {
		t.Run(flag, func(t *testing.T) {
			os.Args = []string{"upm", flag, "version"}

			// Redirect stdout
			oldStdout := os.Stdout
			os.Stdout, _ = os.Open(os.DevNull)
			defer func() { os.Stdout = oldStdout }()

			code := Run()
			if code != 0 {
				t.Errorf("Run() with %s version should return 0, got %d", flag, code)
			}
		})
	}
}


package scripts

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func scriptNames(defs []ScriptDefinition) []string {
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	return names
}

func TestLoadPyprojectTOML(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
		cmd  string
	}{
		{"xpm only", "[tool.xpm.scripts]\ntest = \"pytest\"\n", []string{"test"}, "pytest"},
		{"upm fallback", "[tool.upm.scripts]\ntest = \"pytest\"\n", []string{"test"}, "pytest"},
		{"xpm wins over upm", "[tool.xpm.scripts]\na = \"x\"\n[tool.upm.scripts]\nb = \"y\"\n", []string{"a"}, "x"},
		{"poetry only", "[tool.poetry.scripts]\nrun = \"pkg:main\"\n", []string{"run"}, "pkg:main"},
		{"project only", "[project.scripts]\ncli = \"pkg:cli\"\n", []string{"cli"}, "pkg:cli"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pyproject.toml")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := loadPyprojectTOML(path)
			if err != nil {
				t.Fatal(err)
			}
			names := scriptNames(got)
			if len(names) != len(tt.want) || names[0] != tt.want[0] {
				t.Fatalf("scripts = %v, want %v", names, tt.want)
			}
			if got[0].Command != tt.cmd {
				t.Errorf("command = %q, want %q", got[0].Command, tt.cmd)
			}
		})
	}
}

func TestLoadCargoTOML(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
		cmd  string
	}{
		{"xpm only", "[package.metadata.xpm.scripts]\ntest = \"cargo test\"\n", "test", "cargo test"},
		{"upm fallback", "[package.metadata.upm.scripts]\ntest = \"cargo test\"\n", "test", "cargo test"},
		{"xpm wins over upm", "[package.metadata.xpm.scripts]\na = \"x\"\n[package.metadata.upm.scripts]\nb = \"y\"\n", "a", "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Cargo.toml")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := loadCargoTOML(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].Name != tt.want || got[0].Command != tt.cmd {
				t.Fatalf("scripts = %+v, want one %q=%q", got, tt.want, tt.cmd)
			}
		})
	}
}

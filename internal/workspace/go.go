package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectGoWorkspace detects the modules listed by go.work "use" directives
// (single-line and block forms). Without a go.work it falls back to the
// go.mod files below root (not root itself), skipping vendor/, testdata/,
// hidden directories and the other skipDir names.
func DetectGoWorkspace(root string) (*Workspace, error) {
	goWork := filepath.Join(root, "go.work")
	data, err := os.ReadFile(goWork)
	if err != nil {
		return detectGoModules(root)
	}
	wf, err := modfile.ParseWork(goWork, data, nil)
	if err != nil {
		return nil, fmt.Errorf("go.work: %w", err)
	}
	var projects []Project
	for _, use := range wf.Use {
		dir := filepath.FromSlash(use.Path)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		if p, ok := goProject(dir); ok {
			projects = append(projects, p)
		}
	}
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "go"}, nil
}

// detectGoModules finds go.mod files below root; nested modules inside a
// found module are not separate projects.
func detectGoModules(root string) (*Workspace, error) {
	var projects []Project
	walkDirs(root, func(dir string) bool {
		p, ok := goProject(dir)
		if ok {
			projects = append(projects, p)
		}
		return ok
	})
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "go"}, nil
}

// goProject describes the module in dir, named by its go.mod module path.
func goProject(dir string) (Project, bool) {
	manifest := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(manifest)
	if err != nil {
		return Project{}, false
	}
	name := modfile.ModulePath(data)
	if name == "" {
		name = filepath.Base(dir)
	}
	lockfile := ""
	if p := filepath.Join(dir, "go.sum"); isFile(p) {
		lockfile = p
	}
	return Project{Name: name, Path: filepath.Clean(dir), Ecosystem: "go",
		Manifest: manifest, Lockfile: lockfile, PM: pm.GoMod}, true
}

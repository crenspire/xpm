package workspace

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectGoWorkspace detects Go workspaces (go.work).
func DetectGoWorkspace(root string) (*Workspace, error) {
	goWorkPath := filepath.Join(root, "go.work")
	if _, err := os.Stat(goWorkPath); err != nil {
		// Fallback: scan for directories with go.mod files
		return detectGoModules(root)
	}

	// Parse go.work file
	file, err := os.Open(goWorkPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var projects []Project
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Look for "use ./path" directives
		if strings.HasPrefix(line, "use ") {
			path := strings.TrimPrefix(line, "use ")
			path = strings.TrimSpace(path)
			// Remove quotes if present
			path = strings.Trim(path, `"`)
			path = strings.Trim(path, "'")

			// Resolve path relative to root
			projectPath := filepath.Join(root, path)
			if !filepath.IsAbs(path) {
				projectPath = filepath.Join(root, path)
			}

			// Check if go.mod exists
			goModPath := filepath.Join(projectPath, "go.mod")
			if _, err := os.Stat(goModPath); err != nil {
				continue
			}

			// Read go.mod to get module name
			moduleName := readGoModuleName(goModPath)
			if moduleName == "" {
				moduleName = filepath.Base(projectPath)
			}

			// Check for go.sum
			goSumPath := filepath.Join(projectPath, "go.sum")
			lockfile := ""
			if _, err := os.Stat(goSumPath); err == nil {
				lockfile = goSumPath
			}

			projects = append(projects, Project{
				Name:      moduleName,
				Path:      projectPath,
				Ecosystem: "go",
				Manifest:  goModPath,
				Lockfile:  lockfile,
				PM:        pm.GoMod,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(projects) == 0 {
		return nil, nil
	}

	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "go",
	}, nil
}

// detectGoModules scans for directories with go.mod files (fallback).
func detectGoModules(root string) (*Workspace, error) {
	var projects []Project

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip root
		if path == root {
			return nil
		}

		// Only check directories
		if !info.IsDir() {
			return nil
		}

		// Check for go.mod
		goModPath := filepath.Join(path, "go.mod")
		if _, err := os.Stat(goModPath); err != nil {
			return nil
		}

		// Skip if already in projects (parent directory)
		for _, p := range projects {
			if strings.HasPrefix(path, p.Path+string(filepath.Separator)) {
				return filepath.SkipDir
			}
		}

		moduleName := readGoModuleName(goModPath)
		if moduleName == "" {
			moduleName = filepath.Base(path)
		}

		goSumPath := filepath.Join(path, "go.sum")
		lockfile := ""
		if _, err := os.Stat(goSumPath); err == nil {
			lockfile = goSumPath
		}

		projects = append(projects, Project{
			Name:      moduleName,
			Path:      path,
			Ecosystem: "go",
			Manifest:  goModPath,
			Lockfile:  lockfile,
			PM:        pm.GoMod,
		})

		return filepath.SkipDir // Don't recurse into subdirectories
	})

	if err != nil {
		return nil, err
	}

	if len(projects) == 0 {
		return nil, nil
	}

	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "go",
	}, nil
}

// readGoModuleName reads the module name from go.mod.
func readGoModuleName(goModPath string) string {
	file, err := os.Open(goModPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			moduleName := strings.TrimPrefix(line, "module ")
			moduleName = strings.TrimSpace(moduleName)
			return moduleName
		}
	}

	return ""
}

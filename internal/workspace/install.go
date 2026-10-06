package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/crenspire/xpm/internal/pm"
)

// InstallWorkspaces installs dependencies across all projects in workspaces.
func InstallWorkspaces(workspaces []Workspace, parallel bool) error {
	if len(workspaces) == 0 {
		return fmt.Errorf("no workspaces provided")
	}

	var allProjects []Project
	for _, ws := range workspaces {
		allProjects = append(allProjects, ws.Projects...)
	}

	if len(allProjects) == 0 {
		return fmt.Errorf("no projects found in workspaces")
	}

	if parallel {
		return installProjectsParallel(allProjects)
	}

	return installProjectsSequential(allProjects)
}

// installProjectsParallel installs dependencies in parallel.
func installProjectsParallel(projects []Project) error {
	var wg sync.WaitGroup
	errors := make(chan error, len(projects))

	for _, project := range projects {
		wg.Add(1)
		go func(p Project) {
			defer wg.Done()
			if err := installProject(p); err != nil {
				errors <- fmt.Errorf("[%s] %v", p.Name, err)
			}
		}(project)
	}

	wg.Wait()
	close(errors)

	// Collect errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors in %d projects: %v", len(errs), errs[0])
	}

	return nil
}

// installProjectsSequential installs dependencies sequentially.
func installProjectsSequential(projects []Project) error {
	for _, project := range projects {
		if err := installProject(project); err != nil {
			return fmt.Errorf("[%s] %v", project.Name, err)
		}
	}
	return nil
}

// installProject installs dependencies for a single project.
func installProject(project Project) error {
	// Use detected PM or detect from lockfiles
	pmID := project.PM
	if pmID == "" {
		lockFiles := pm.DetectLockFiles(project.Path)
		// Try to determine PM from lockfiles
		for _, pms := range lockFiles {
			if len(pms) > 0 {
				pmID = pms[0]
				break
			}
		}
	}

	if pmID == "" {
		// Try to infer from ecosystem
		switch project.Ecosystem {
		case "node":
			pmID = pm.Npm
		case "python":
			pmID = pm.Pip
		case "rust":
			pmID = pm.Cargo
		case "go":
			pmID = pm.GoMod
		case "java":
			pmID = pm.Maven
		case "php":
			pmID = pm.Composer
		default:
			return fmt.Errorf("cannot determine package manager for ecosystem %s", project.Ecosystem)
		}
	}

	meta, ok := pm.MetaFor(pmID)
	if !ok {
		return fmt.Errorf("unknown package manager: %s", pmID)
	}

	if !pm.Exists(meta.Binary) {
		return fmt.Errorf("package manager %s (%s) is not installed", meta.Name, meta.Binary)
	}

	// Change to project directory
	originalDir, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(project.Path); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	fmt.Printf("[%s] Installing dependencies via %s...\n", project.Name, meta.Name)

	// Run install command based on package manager
	var installArgs []string
	switch pmID {
	case pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun:
		installArgs = []string{"install"}
	case pm.Pip, pm.Poetry, pm.Pipenv:
		installArgs = []string{"install"}
	case pm.Composer:
		installArgs = []string{"install"}
	case pm.Cargo:
		installArgs = []string{"build"} // Cargo doesn't have a separate install command
	case pm.GoMod:
		installArgs = []string{"mod", "download"}
	case pm.Maven:
		installArgs = []string{"install"}
	case pm.Gradle:
		installArgs = []string{"build"}
	default:
		return fmt.Errorf("unsupported package manager: %s", pmID)
	}

	// Execute the install command
	cmd := exec.Command(meta.Binary, installArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Printf("[%s] ✓ Installed\n", project.Name)
	return nil
}

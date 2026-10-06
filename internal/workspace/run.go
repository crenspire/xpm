package workspace

import (
	"fmt"
	"os"
	"sync"

	"github.com/crenspire/xpm/internal/scripts"
)

// RunInWorkspaces runs a task across all projects in workspaces.
func RunInWorkspaces(workspaces []Workspace, task string, parallel bool) error {
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
		return runInProjectsParallel(allProjects, task)
	}

	return runInProjectsSequential(allProjects, task)
}

// runInProjectsParallel runs tasks in parallel using goroutines.
func runInProjectsParallel(projects []Project, task string) error {
	var wg sync.WaitGroup
	errors := make(chan error, len(projects))

	for _, project := range projects {
		wg.Add(1)
		go func(p Project) {
			defer wg.Done()
			if err := runInProject(p, task); err != nil {
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

// runInProjectsSequential runs tasks sequentially.
func runInProjectsSequential(projects []Project, task string) error {
	for _, project := range projects {
		if err := runInProject(project, task); err != nil {
			return fmt.Errorf("[%s] %v", project.Name, err)
		}
	}
	return nil
}

// runInProject runs a task in a single project.
func runInProject(project Project, task string) error {
	// Check if task exists in project
	merged, err := scripts.LoadAllScripts(project.Path, nil)
	if err != nil {
		return fmt.Errorf("failed to load scripts: %w", err)
	}

	script, found := merged.GetScript(task)
	if !found {
		return fmt.Errorf("task %q not found", task)
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

	fmt.Printf("[%s] Running %s...\n", project.Name, task)

	// Run the script
	if err := scripts.RunScript(*script, nil); err != nil {
		return fmt.Errorf("task execution failed: %w", err)
	}

	fmt.Printf("[%s] ✓ Completed\n", project.Name)
	return nil
}

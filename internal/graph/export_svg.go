package graph

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// CheckGraphViz checks if GraphViz (dot) is installed.
func CheckGraphViz() bool {
	_, err := exec.LookPath("dot")
	return err == nil
}

// GenerateSVG generates an SVG file from a DOT string using GraphViz.
func GenerateSVG(dotContent string, outputPath string) error {
	if !CheckGraphViz() {
		return fmt.Errorf("GraphViz (dot) is not installed. Install it to generate SVG output")
	}

	// Create temporary DOT file
	tmpDir := os.TempDir()
	tmpDOT := filepath.Join(tmpDir, "graph.dot")
	defer os.Remove(tmpDOT)

	if err := os.WriteFile(tmpDOT, []byte(dotContent), 0644); err != nil {
		return fmt.Errorf("failed to write temporary DOT file: %w", err)
	}

	// Run dot command
	cmd := exec.Command("dot", "-Tsvg", tmpDOT, "-o", outputPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate SVG: %w", err)
	}

	return nil
}

// WriteSVG writes the graph as SVG to the writer.
func WriteSVG(graph *DepGraph, w io.Writer) error {
	if !CheckGraphViz() {
		return fmt.Errorf("GraphViz (dot) is not installed. Install it to generate SVG output")
	}

	// Generate DOT
	dotContent := ToDOT(graph)

	// Create temporary files
	tmpDir := os.TempDir()
	tmpDOT := filepath.Join(tmpDir, "graph.dot")
	tmpSVG := filepath.Join(tmpDir, "graph.svg")
	defer os.Remove(tmpDOT)
	defer os.Remove(tmpSVG)

	// Write DOT file
	if err := os.WriteFile(tmpDOT, []byte(dotContent), 0644); err != nil {
		return fmt.Errorf("failed to write temporary DOT file: %w", err)
	}

	// Generate SVG
	cmd := exec.Command("dot", "-Tsvg", tmpDOT, "-o", tmpSVG)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate SVG: %w", err)
	}

	// Read and write SVG
	svgData, err := os.ReadFile(tmpSVG)
	if err != nil {
		return fmt.Errorf("failed to read generated SVG: %w", err)
	}

	_, err = w.Write(svgData)
	return err
}


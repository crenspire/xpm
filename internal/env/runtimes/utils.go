package runtimes

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// extractTarGz extracts a .tar.gz file.
func extractTarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, os.FileMode(header.Mode))
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// extractZip extracts a .zip file.
func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
			continue
		}

		os.MkdirAll(filepath.Dir(path), 0755)
		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

// copyDirectory copies a directory recursively, handling symlinks.
func copyDirectory(src, dest string) error {
	// Ensure destination exists
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	var copyErr error
	var filesCopied int
	err := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			copyErr = fmt.Errorf("walk error at %s: %w", path, walkErr)
			return walkErr
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			copyErr = fmt.Errorf("failed to get relative path for %s: %w", path, err)
			return err
		}

		// Skip the root directory itself (relPath will be ".")
		// We only want to copy its contents
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(dest, relPath)

		// Use Lstat to properly detect symlinks (filepath.Walk uses Lstat, but we need to check explicitly)
		linkInfo, err := os.Lstat(path)
		if err != nil {
			return err
		}

		// Handle symlinks - preserve them as-is since the target should be in the copied structure
		if linkInfo.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// Make sure the destination directory exists
			os.MkdirAll(filepath.Dir(destPath), 0755)
			// Remove existing file/symlink if it exists
			os.Remove(destPath)
			// Create the symlink with the same target (relative paths should work since structure is preserved)
			if err := os.Symlink(linkTarget, destPath); err != nil {
				return fmt.Errorf("failed to create symlink %s -> %s: %w", destPath, linkTarget, err)
			}
			return nil
		}

		if linkInfo.IsDir() {
			return os.MkdirAll(destPath, linkInfo.Mode())
		}

		// Copy regular file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		os.MkdirAll(filepath.Dir(destPath), 0755)
		destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, linkInfo.Mode())
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		if err != nil {
			return err
		}
		
		// Preserve executable permissions
		if linkInfo.Mode()&0111 != 0 {
			os.Chmod(destPath, linkInfo.Mode()|0111)
		}
		
		filesCopied++
		return nil
	})
	
	if err != nil {
		if copyErr != nil {
			return copyErr
		}
		return fmt.Errorf("filepath.Walk failed: %w", err)
	}
	
	if copyErr != nil {
		return copyErr
	}
	
	if filesCopied == 0 {
		return fmt.Errorf("no files were copied from %s to %s", src, dest)
	}
	
	return nil
}


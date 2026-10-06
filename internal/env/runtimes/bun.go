package runtimes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crenspire/xpm/internal/env"
)

// BunInstaller installs Bun from GitHub releases, verified against SHASUMS256.txt.
type BunInstaller struct{}

func init() {
	env.RegisterInstaller("bun", &BunInstaller{})
}

// Name returns the runtime name.
func (b *BunInstaller) Name() string { return "bun" }

// ListRemote returns versions from release tags like "bun-v1.4.2".
func (b *BunInstaller) ListRemote(ctx context.Context) ([]string, error) {
	rels, err := fetchGitHubReleases(ctx, "oven-sh/bun")
	if err != nil {
		return nil, err
	}
	return versionsFromTags(rels, "bun-v"), nil
}

// bunAsset names the release zip and its top directory for a platform.
func bunAsset(goos, goarch string) (zip, dir string, err error) {
	osName := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "windows"}[goos]
	arch := map[string]string{"amd64": "x64", "arm64": "aarch64"}[goarch]
	if osName == "" || arch == "" {
		return "", "", fmt.Errorf("bun publishes no binaries for %s/%s", goos, goarch)
	}
	dir = fmt.Sprintf("bun-%s-%s", osName, arch)
	return dir + ".zip", dir, nil
}

// Install downloads, verifies and unpacks Bun into req.Dest/bin, and adds
// bin/bunx -> bun (bun acts as bunx when invoked under that name).
func (b *BunInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	zip, dir, err := bunAsset(hostOS, hostArch)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("%s/oven-sh/bun/releases/download/bun-v%s", githubDownloadURL, req.Version)
	sums, err := fetchSmall(ctx, base+"/SHASUMS256.txt")
	if err != nil {
		return fmt.Errorf("fetch bun checksums: %w", err)
	}
	want, err := checksumFromSums(string(sums), zip)
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, base+"/"+zip, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	if err := moveToBin(req.Dest, dir+"/bun", "bun"); err != nil {
		return fmt.Errorf("bun archive layout changed: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(req.Dest, dir)); err != nil {
		return err
	}
	return os.Symlink("bun", filepath.Join(req.Dest, "bin", "bunx"))
}

// BinaryPaths returns the Bun binaries.
func (b *BunInstaller) BinaryPaths() []string { return []string{"bin/bun", "bin/bunx"} }

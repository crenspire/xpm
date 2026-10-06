package runtimes

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// DenoInstaller installs Deno from GitHub releases, verified against the
// per-asset .sha256sum file.
type DenoInstaller struct{}

func init() {
	env.RegisterInstaller("deno", &DenoInstaller{})
}

// Name returns the runtime name.
func (d *DenoInstaller) Name() string { return "deno" }

// ListRemote returns versions from release tags like "v2.9.7".
func (d *DenoInstaller) ListRemote(ctx context.Context) ([]string, error) {
	rels, err := fetchGitHubReleases(ctx, "denoland/deno")
	if err != nil {
		return nil, err
	}
	return versionsFromTags(rels, "v"), nil
}

// denoAsset names the release zip for a platform.
func denoAsset(goos, goarch string) (string, error) {
	triple := map[string]string{"darwin": "apple-darwin", "linux": "unknown-linux-gnu", "windows": "pc-windows-msvc"}[goos]
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	if triple == "" || arch == "" {
		return "", fmt.Errorf("deno publishes no binaries for %s/%s", goos, goarch)
	}
	return fmt.Sprintf("deno-%s-%s.zip", arch, triple), nil
}

// parseSumFile reads "<hex>  <name>" (or a bare "<hex>").
func parseSumFile(body, name string) (string, error) {
	fields := strings.Fields(body)
	if len(fields) == 1 {
		return fields[0], nil
	}
	return checksumFromSums(body, name)
}

// Install downloads, verifies and unpacks Deno; its zip holds a root-level
// deno binary, which moves to bin/deno.
func (d *DenoInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	zip, err := denoAsset(hostOS, hostArch)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/denoland/deno/releases/download/v%s/%s", githubDownloadURL, req.Version, zip)
	body, err := fetchSmall(ctx, url+".sha256sum")
	if isNotFound(err) {
		return fmt.Errorf("deno %s publishes no SHA-256 checksum for %s; xpm only installs verified downloads (Deno 2.0.6+ publish them)", req.Version, zip)
	}
	if err != nil {
		return fmt.Errorf("fetch deno checksum: %w", err)
	}
	want, err := parseSumFile(string(body), zip)
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, url, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	if err := moveToBin(req.Dest, "deno", "deno"); err != nil {
		return fmt.Errorf("deno archive has no deno binary at its root: %w", err)
	}
	return nil
}

// BinaryPaths returns the Deno binary.
func (d *DenoInstaller) BinaryPaths() []string { return []string{"bin/deno"} }

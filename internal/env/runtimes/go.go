package runtimes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// GoInstaller installs Go from go.dev, verified against the release feed.
type GoInstaller struct{}

func init() {
	env.RegisterInstaller("go", &GoInstaller{})
}

// Name returns the runtime name.
func (g *GoInstaller) Name() string { return "go" }

// ListRemote returns every stable Go version (the include=all feed).
func (g *GoInstaller) ListRemote(ctx context.Context) ([]string, error) {
	data, err := fetchSmall(ctx, goDLURL+"/?mode=json&include=all")
	if err != nil {
		return nil, err
	}
	return stableGoVersions(data)
}

// stableGoVersions parses the go.dev release feed and returns the stable
// versions (without the "go" prefix) in feed order, de-duplicated. Betas and
// release candidates are skipped.
func stableGoVersions(releasesJSON []byte) ([]string, error) {
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.Unmarshal(releasesJSON, &releases); err != nil {
		return nil, err
	}
	var versions []string
	seen := make(map[string]bool)
	for _, release := range releases {
		if !release.Stable {
			continue
		}
		version := strings.TrimPrefix(release.Version, "go")
		if !seen[version] {
			versions = append(versions, version)
			seen[version] = true
		}
	}
	return versions, nil
}

// goAsset names the Go archive for a platform (Windows ships .zip).
func goAsset(goos, goarch, version string) string {
	if goarch == "arm" {
		goarch = "armv6l"
	}
	if goos == "windows" {
		return fmt.Sprintf("go%s.%s-%s.zip", version, goos, goarch)
	}
	return fmt.Sprintf("go%s.%s-%s.tar.gz", version, goos, goarch)
}

// Install downloads, verifies and unpacks Go into req.Dest.
func (g *GoInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	file := goAsset(hostOS, hostArch, req.Version)
	want, err := goReleaseChecksum(ctx, file)
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, goDLURL+"/"+file, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	return hoistDir(req.Dest, "go")
}

// BinaryPaths returns the Go binaries.
func (g *GoInstaller) BinaryPaths() []string { return []string{"bin/go", "bin/gofmt"} }

// goReleaseChecksum looks the archive up in the small current-releases feed
// first and only falls back to the full (include=all) feed when absent.
// It fails closed: no checksum means an error.
func goReleaseChecksum(ctx context.Context, filename string) (string, error) {
	var lastErr error
	for _, u := range []string{goDLURL + "/?mode=json", goDLURL + "/?mode=json&include=all"} {
		meta, err := fetchSmall(ctx, u)
		if err != nil {
			lastErr = fmt.Errorf("fetch Go release list: %w", err)
			continue
		}
		want, err := goChecksum(meta, filename)
		if err != nil {
			lastErr = err
			continue
		}
		return want, nil
	}
	return "", lastErr
}

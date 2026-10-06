package runtimes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// NodeInstaller installs Node.js from nodejs.org, verified against SHASUMS256.txt.
type NodeInstaller struct{}

func init() {
	env.RegisterInstaller("node", &NodeInstaller{})
}

// Name returns the runtime name.
func (n *NodeInstaller) Name() string { return "node" }

// nodeRelease is one entry of nodejs.org/dist/index.json.
type nodeRelease struct {
	Version string `json:"version"`
	LTS     any    `json:"lts"` // false, or the LTS codename
}

func (n *NodeInstaller) index(ctx context.Context) ([]nodeRelease, error) {
	var releases []nodeRelease
	if err := fetchJSON(ctx, nodeDistURL+"/index.json", &releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// ListRemote returns every published Node.js version.
func (n *NodeInstaller) ListRemote(ctx context.Context) ([]string, error) {
	releases, err := n.index(ctx)
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(releases))
	for _, r := range releases {
		versions = append(versions, strings.TrimPrefix(r.Version, "v"))
	}
	return versions, nil
}

// LatestLTS returns the newest LTS release (index.json is newest first).
func (n *NodeInstaller) LatestLTS(ctx context.Context) (string, error) {
	releases, err := n.index(ctx)
	if err != nil {
		return "", err
	}
	for _, r := range releases {
		switch v := r.LTS.(type) {
		case bool:
			if v {
				return strings.TrimPrefix(r.Version, "v"), nil
			}
		case string:
			if v != "" {
				return strings.TrimPrefix(r.Version, "v"), nil
			}
		}
	}
	return "", errors.New("nodejs.org lists no LTS release")
}

// nodeAsset names the archive and its top directory for a platform.
func nodeAsset(goos, goarch, version string) (file, dir string, err error) {
	osName := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "win"}[goos]
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if osName == "" || arch == "" {
		return "", "", fmt.Errorf("nodejs.org publishes no Node.js binaries for %s/%s", goos, goarch)
	}
	dir = fmt.Sprintf("node-v%s-%s-%s", version, osName, arch)
	if goos == "windows" {
		return dir + ".zip", dir, nil
	}
	return dir + ".tar.gz", dir, nil
}

// Install downloads, verifies and unpacks Node.js into req.Dest.
func (n *NodeInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	file, dir, err := nodeAsset(hostOS, hostArch, req.Version)
	if err != nil {
		return err
	}
	base := nodeDistURL + "/v" + req.Version
	sums, err := fetchSmall(ctx, base+"/SHASUMS256.txt")
	if err != nil {
		return fmt.Errorf("fetch Node.js checksums: %w", err)
	}
	want, err := checksumFromSums(string(sums), file)
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, base+"/"+file, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	return hoistDir(req.Dest, dir)
}

// BinaryPaths: corepack is not shipped from Node 25 on, so it is not listed.
func (n *NodeInstaller) BinaryPaths() []string {
	return []string{"bin/node", "bin/npm", "bin/npx"}
}

package runtimes

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// pbsLatestURL describes the newest python-build-standalone release.
var pbsLatestURL = "https://raw.githubusercontent.com/astral-sh/python-build-standalone/latest-release/latest-release.json"

// PythonInstaller installs CPython builds from python-build-standalone
// (the builds uv and rye use), verified against the release's SHA256SUMS.
type PythonInstaller struct{}

func init() {
	env.RegisterInstaller("python", &PythonInstaller{})
}

// Name returns the runtime name.
func (p *PythonInstaller) Name() string { return "python" }

// pbsRelease is one python-build-standalone release and its checksums.
type pbsRelease struct {
	Tag    string
	Prefix string // asset URL prefix
	Sums   string // SHA256SUMS content
	Triple string
}

// pbsTriple maps GOOS/GOARCH to the supported build triples.
func pbsTriple(goos, goarch string) (string, error) {
	t := map[string]string{
		"darwin/arm64": "aarch64-apple-darwin",
		"darwin/amd64": "x86_64-apple-darwin",
		"linux/amd64":  "x86_64-unknown-linux-gnu",
		"linux/arm64":  "aarch64-unknown-linux-gnu",
	}[goos+"/"+goarch]
	if t == "" {
		return "", fmt.Errorf("xpm installs Python only on macOS and Linux (x86_64, arm64), not %s/%s", goos, goarch)
	}
	return t, nil
}

func (p *PythonInstaller) latest(ctx context.Context) (pbsRelease, error) {
	triple, err := pbsTriple(hostOS, hostArch)
	if err != nil {
		return pbsRelease{}, err
	}
	var meta struct {
		Tag            string `json:"tag"`
		AssetURLPrefix string `json:"asset_url_prefix"`
	}
	if err := fetchJSON(ctx, pbsLatestURL, &meta); err != nil {
		return pbsRelease{}, err
	}
	if meta.Tag == "" || meta.AssetURLPrefix == "" {
		return pbsRelease{}, fmt.Errorf("unexpected python-build-standalone release info from %s", pbsLatestURL)
	}
	sums, err := fetchSmall(ctx, strings.TrimSuffix(meta.AssetURLPrefix, "/")+"/SHA256SUMS")
	if err != nil {
		return pbsRelease{}, fmt.Errorf("fetch python-build-standalone checksums: %w", err)
	}
	return pbsRelease{Tag: meta.Tag, Prefix: strings.TrimSuffix(meta.AssetURLPrefix, "/"), Sums: string(sums), Triple: triple}, nil
}

// pbsAsset names the install_only archive of version in a release.
func pbsAsset(version, tag, triple string) string {
	return fmt.Sprintf("cpython-%s+%s-%s-install_only.tar.gz", version, tag, triple)
}

// versions lists the CPython versions this release offers for its triple
// (only the plain install_only flavour).
func (r pbsRelease) versions() []string {
	suffix := "+" + r.Tag + "-" + r.Triple + "-install_only.tar.gz"
	var out []string
	for _, line := range strings.Split(r.Sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if !strings.HasPrefix(name, "cpython-") || !strings.HasSuffix(name, suffix) {
			continue
		}
		v := strings.TrimSuffix(strings.TrimPrefix(name, "cpython-"), suffix)
		if env.ValidateVersionSpec(v) == nil {
			out = append(out, v)
		}
	}
	return out
}

// ListRemote returns the versions of the newest release for this platform.
func (p *PythonInstaller) ListRemote(ctx context.Context) ([]string, error) {
	r, err := p.latest(ctx)
	if err != nil {
		return nil, err
	}
	return r.versions(), nil
}

// Resolve picks from the newest release only: "latest" or a partial
// version selects the highest stable match; an exact version must be offered.
func (p *PythonInstaller) Resolve(ctx context.Context, spec string) (string, error) {
	r, err := p.latest(ctx)
	if err != nil {
		return "", err
	}
	versions := r.versions()
	offered := append([]string(nil), versions...)
	env.SortVersionsDesc(offered)
	for i, j := 0, len(offered)-1; i < j; i, j = i+1, j-1 {
		offered[i], offered[j] = offered[j], offered[i]
	}
	list := strings.Join(offered, ", ")
	pv, ok := env.ParseVersion(spec)
	if spec != "latest" && ok && len(pv.Nums) >= 3 {
		for _, v := range versions {
			if v == spec {
				return v, nil
			}
		}
		return "", fmt.Errorf("python %s is not available: python-build-standalone %s provides %s", spec, r.Tag, list)
	}
	if v, found := env.HighestMatch(spec, versions, false); found {
		return v, nil
	}
	return "", fmt.Errorf("no python version matches %s: python-build-standalone %s provides %s", spec, r.Tag, list)
}

// Install downloads, verifies and unpacks CPython into req.Dest.
func (p *PythonInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	r, err := p.latest(ctx)
	if err != nil {
		return err
	}
	asset := pbsAsset(req.Version, r.Tag, r.Triple)
	want, err := checksumFromSums(r.Sums, asset)
	if err != nil {
		return fmt.Errorf("python %s is not in python-build-standalone %s: %w", req.Version, r.Tag, err)
	}
	archive, err := downloadVerified(ctx, r.Prefix+"/"+asset, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	return hoistDir(req.Dest, "python")
}

// BinaryPaths returns the Python binaries.
func (p *PythonInstaller) BinaryPaths() []string {
	return []string{"bin/python", "bin/python3", "bin/pip", "bin/pip3"}
}

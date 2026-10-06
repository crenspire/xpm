package runtimes

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// adoptiumAPI is the Eclipse Temurin (Adoptium) API.
var adoptiumAPI = "https://api.adoptium.net"

var javaMajorRe = regexp.MustCompile(`^[0-9]+$`)

// JavaInstaller installs Eclipse Temurin JDKs via the Adoptium API,
// verified against the checksum the API publishes for each package.
type JavaInstaller struct{}

func init() {
	env.RegisterInstaller("java", &JavaInstaller{})
}

// Name returns the runtime name.
func (j *JavaInstaller) Name() string { return "java" }

type adoptiumReleases struct {
	AvailableReleases []int `json:"available_releases"`
	MostRecentFeature int   `json:"most_recent_feature_release"`
	MostRecentLTS     int   `json:"most_recent_lts"`
}

type adoptiumPackage struct {
	Checksum string `json:"checksum"`
	Link     string `json:"link"`
	Name     string `json:"name"`
}

func (j *JavaInstaller) available(ctx context.Context) (adoptiumReleases, error) {
	var r adoptiumReleases
	err := fetchJSON(ctx, adoptiumAPI+"/v3/info/available_releases", &r)
	return r, err
}

// ListRemote returns the available feature releases (majors: "21", "17", ...).
func (j *JavaInstaller) ListRemote(ctx context.Context) ([]string, error) {
	r, err := j.available(ctx)
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(r.AvailableReleases))
	for _, v := range r.AvailableReleases {
		versions = append(versions, strconv.Itoa(v))
	}
	return versions, nil
}

// adoptiumPlatform maps Go's GOOS/GOARCH to Adoptium's os/architecture.
func adoptiumPlatform(goos, goarch string) (osName, arch string, err error) {
	osName = map[string]string{"darwin": "mac", "linux": "linux", "windows": "windows"}[goos]
	arch = map[string]string{"amd64": "x64", "arm64": "aarch64"}[goarch]
	if osName == "" || arch == "" {
		return "", "", fmt.Errorf("adoptium publishes no JDK for %s/%s", goos, goarch)
	}
	return osName, arch, nil
}

// javaVersionFromRelease turns "jdk-21.0.12.1+1" into "21.0.12.1+1" and
// "jdk8u422-b05" into "8u422-b05".
func javaVersionFromRelease(name string) string {
	if v, ok := strings.CutPrefix(name, "jdk-"); ok {
		return v
	}
	return strings.TrimPrefix(name, "jdk")
}

// javaReleaseName is the inverse of javaVersionFromRelease.
func javaReleaseName(version string) string {
	if strings.HasPrefix(version, "8u") {
		return "jdk" + version
	}
	return "jdk-" + version
}

// Resolve maps "latest" (newest feature release), "lts" (newest LTS) and a
// major ("21") to that major's newest exact release; anything else that
// starts with a digit is taken as an exact release ("21.0.4+7").
func (j *JavaInstaller) Resolve(ctx context.Context, spec string) (string, error) {
	major := spec
	switch {
	case spec == "latest" || spec == "lts":
		r, err := j.available(ctx)
		if err != nil {
			return "", err
		}
		n := r.MostRecentFeature
		if spec == "lts" {
			n = r.MostRecentLTS
		}
		if n == 0 {
			return "", fmt.Errorf("adoptium did not report a %s release", spec)
		}
		major = strconv.Itoa(n)
	case javaMajorRe.MatchString(spec):
	case spec[0] >= '0' && spec[0] <= '9':
		return spec, nil
	default:
		return "", fmt.Errorf("unknown java version %q: use a major (21), latest, lts or an exact release (21.0.4+7)", spec)
	}
	osName, arch, err := adoptiumPlatform(hostOS, hostArch)
	if err != nil {
		return "", err
	}
	q := url.Values{"architecture": {arch}, "image_type": {"jdk"}, "os": {osName}, "vendor": {"eclipse"}}
	var assets []struct {
		ReleaseName string `json:"release_name"`
	}
	if err := fetchJSON(ctx, fmt.Sprintf("%s/v3/assets/latest/%s/hotspot?%s", adoptiumAPI, major, q.Encode()), &assets); err != nil {
		return "", err
	}
	if len(assets) == 0 || assets[0].ReleaseName == "" {
		return "", fmt.Errorf("adoptium has no JDK %s for %s/%s", major, osName, arch)
	}
	return javaVersionFromRelease(assets[0].ReleaseName), nil
}

// Install downloads, verifies and unpacks the JDK. The archive's single top
// directory is hoisted; on macOS the JDK home (Contents/Home) becomes the root.
func (j *JavaInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	osName, arch, err := adoptiumPlatform(hostOS, hostArch)
	if err != nil {
		return err
	}
	q := url.Values{"architecture": {arch}, "heap_size": {"normal"}, "image_type": {"jdk"}, "jvm_impl": {"hotspot"}, "os": {osName}}
	name := strings.ReplaceAll(url.PathEscape(javaReleaseName(req.Version)), "+", "%2B")
	var release struct {
		Binaries []struct {
			Package adoptiumPackage `json:"package"`
		} `json:"binaries"`
	}
	if err := fetchJSON(ctx, fmt.Sprintf("%s/v3/assets/release_name/eclipse/%s?%s", adoptiumAPI, name, q.Encode()), &release); err != nil {
		if isNotFound(err) {
			return fmt.Errorf("adoptium has no release %s (see: xpm env ls-remote java)", javaReleaseName(req.Version))
		}
		return err
	}
	if len(release.Binaries) == 0 || release.Binaries[0].Package.Link == "" {
		return fmt.Errorf("adoptium has no %s JDK package for %s/%s", javaReleaseName(req.Version), osName, arch)
	}
	pkg := release.Binaries[0].Package
	archive, err := downloadVerified(ctx, pkg.Link, pkg.Checksum)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	top, err := singleTopDir(req.Dest)
	if err != nil {
		return err
	}
	if err := hoistDir(req.Dest, top); err != nil {
		return err
	}
	if fi, err := os.Stat(filepath.Join(req.Dest, "Contents", "Home")); err == nil && fi.IsDir() {
		return hoistDir(req.Dest, "Contents/Home")
	}
	return nil
}

// BinaryPaths returns the JDK binaries.
func (j *JavaInstaller) BinaryPaths() []string {
	return []string{"bin/java", "bin/javac", "bin/jar", "bin/keytool"}
}

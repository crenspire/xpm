package runtimes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// phpReleasesURL is php.net's release API.
var phpReleasesURL = "https://www.php.net/releases/index.php"

var (
	phpMajorRe = regexp.MustCompile(`^[0-9]+$`)
	phpMinorRe = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
	phpPatchRe = regexp.MustCompile(`^([0-9]+\.[0-9]+)\.[0-9]+`)
)

// errPHPNeedsBrew is returned wherever xpm cannot install PHP itself.
var errPHPNeedsBrew = errors.New("PHP needs Homebrew on macOS (https://brew.sh); on Linux install PHP with your system package manager")

// PHPInstaller links Homebrew's shivammathur/php builds, one per minor
// version ("8.3"). macOS only.
type PHPInstaller struct{}

func init() {
	env.RegisterInstaller("php", &PHPInstaller{})
}

// Name returns the runtime name.
func (p *PHPInstaller) Name() string { return "php" }

// latestPHPMinor asks php.net for the newest release of a major ("8" -> "8.5").
func latestPHPMinor(ctx context.Context, major string) (string, error) {
	var r struct {
		Version string `json:"version"`
	}
	if err := fetchJSON(ctx, phpReleasesURL+"?json&version="+major, &r); err != nil {
		return "", err
	}
	m := phpPatchRe.FindStringSubmatch(r.Version)
	if m == nil {
		return "", fmt.Errorf("php.net reported no release for PHP %s", major)
	}
	return m[1], nil
}

// Resolve: "latest" or a major -> its newest minor; "8.3" as-is; a patch
// version is refused (Homebrew installs one build per minor).
func (p *PHPInstaller) Resolve(ctx context.Context, spec string) (string, error) {
	switch {
	case spec == "latest":
		return latestPHPMinor(ctx, "8")
	case phpMajorRe.MatchString(spec):
		return latestPHPMinor(ctx, spec)
	case phpMinorRe.MatchString(spec):
		return spec, nil
	}
	if m := phpPatchRe.FindStringSubmatch(spec); m != nil {
		return "", fmt.Errorf("PHP is installed per minor version through Homebrew; use php@%s", m[1])
	}
	return "", fmt.Errorf("unknown PHP version %q: use latest, 8 or 8.3", spec)
}

// ListRemote returns 8.<latest> down to 8.0, then 7.4.
func (p *PHPInstaller) ListRemote(ctx context.Context) ([]string, error) {
	latest, err := latestPHPMinor(ctx, "8")
	if err != nil {
		return nil, err
	}
	_, minorStr, _ := strings.Cut(latest, ".")
	minor, err := strconv.Atoi(minorStr)
	if err != nil {
		return nil, fmt.Errorf("php.net reported %q", latest)
	}
	var versions []string
	for m := minor; m >= 0; m-- {
		versions = append(versions, "8."+strconv.Itoa(m))
	}
	return append(versions, "7.4"), nil
}

// Install runs `brew install shivammathur/php/php@X.Y` and links
// req.Dest/bin/php to that formula's php (no copy, no wrapper scripts).
func (p *PHPInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	if hostOS != "darwin" {
		return errPHPNeedsBrew
	}
	brew, err := lookPath("brew")
	if err != nil {
		return errPHPNeedsBrew
	}
	formula := "shivammathur/php/php@" + req.Version
	if err := runCmd(ctx, brew, []string{"install", formula}, nil); err != nil {
		return fmt.Errorf("brew install %s: %w", formula, err)
	}
	out, err := cmdOutput(ctx, brew, "--prefix", formula)
	if err != nil {
		return fmt.Errorf("brew --prefix %s: %w", formula, err)
	}
	php := filepath.Join(strings.TrimSpace(string(out)), "bin", "php")
	if fi, err := os.Stat(php); err != nil || fi.IsDir() {
		return fmt.Errorf("brew installed %s but %s is missing", formula, php)
	}
	if err := os.MkdirAll(filepath.Join(req.Dest, "bin"), 0o755); err != nil {
		return err
	}
	return os.Symlink(php, filepath.Join(req.Dest, "bin", "php"))
}

// BinaryPaths returns the PHP binary.
func (p *PHPInstaller) BinaryPaths() []string { return []string{"bin/php"} }

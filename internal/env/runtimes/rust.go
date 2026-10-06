package runtimes

import (
	"bufio"
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

var (
	rustDistURL   = "https://static.rust-lang.org/dist"
	rustupDistURL = "https://static.rust-lang.org/rustup/dist"

	rustMinorRe = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
	rustExactRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

// RustInstaller installs Rust toolchains with a private rustup:
// RUSTUP_HOME=<root>/rustup, CARGO_HOME=<root>/cargo. It never touches
// ~/.rustup, ~/.cargo or shell profiles.
type RustInstaller struct{}

func init() {
	env.RegisterInstaller("rust", &RustInstaller{})
}

// Name returns the runtime name.
func (r *RustInstaller) Name() string { return "rust" }

// rustHostTriple maps GOOS/GOARCH to rustup's host triple.
func rustHostTriple(goos, goarch string) (string, error) {
	t := map[string]string{
		"darwin/arm64": "aarch64-apple-darwin",
		"darwin/amd64": "x86_64-apple-darwin",
		"linux/amd64":  "x86_64-unknown-linux-gnu",
		"linux/arm64":  "aarch64-unknown-linux-gnu",
	}[goos+"/"+goarch]
	if t == "" {
		return "", fmt.Errorf("xpm installs Rust only on macOS and Linux (x86_64, arm64), not %s/%s", goos, goarch)
	}
	return t, nil
}

// rustEnv points rustup at xpm's private homes.
func rustEnv(root string) []string {
	return []string{
		"RUSTUP_HOME=" + filepath.Join(root, "rustup"),
		"CARGO_HOME=" + filepath.Join(root, "cargo"),
		"RUSTUP_INIT_SKIP_PATH_CHECK=yes",
	}
}

func rustupPath(root string) string { return filepath.Join(root, "cargo", "bin", "rustup") }

// parseRustChannelVersion reads `version = "1.99.0 (b940084d7 2026-09-28)"`
// from the [pkg.rust] table of a channel-rust-*.toml manifest.
func parseRustChannelVersion(manifest string) (string, error) {
	inRust := false
	sc := bufio.NewScanner(strings.NewReader(manifest))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inRust = line == "[pkg.rust]"
			continue
		}
		if !inRust || !strings.HasPrefix(line, "version") {
			continue
		}
		_, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		s, err := strconv.Unquote(strings.TrimSpace(val))
		if err != nil {
			return "", fmt.Errorf("parse rust channel version %q: %w", val, err)
		}
		if f := strings.Fields(s); len(f) > 0 && rustExactRe.MatchString(f[0]) {
			return f[0], nil
		}
		return "", fmt.Errorf("unexpected rust channel version %q", s)
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", errors.New("rust channel manifest has no [pkg.rust] version")
}

// Resolve: "stable"/"latest" -> current stable; "1.80" -> newest 1.80.x;
// "1.80.1" as-is. Beta and nightly are not supported.
func (r *RustInstaller) Resolve(ctx context.Context, spec string) (string, error) {
	var channel string
	switch {
	case spec == "stable" || spec == "latest":
		channel = "stable"
	case spec == "beta" || spec == "nightly" || strings.HasPrefix(spec, "beta-") || strings.HasPrefix(spec, "nightly-"):
		return "", errors.New("rust channels other than stable are not supported; use an exact version or stable")
	case rustExactRe.MatchString(spec):
		return spec, nil
	case rustMinorRe.MatchString(spec):
		channel = spec
	default:
		return "", fmt.Errorf("unknown rust version %q: use stable, 1.80 or 1.80.1", spec)
	}
	body, err := fetchSmall(ctx, rustDistURL+"/channel-rust-"+channel+".toml")
	if isNotFound(err) {
		return "", fmt.Errorf("no rust release matches %s", spec)
	}
	if err != nil {
		return "", err
	}
	return parseRustChannelVersion(string(body))
}

// ListRemote returns only the current stable version; any 1.x.y released
// since 1.0 can be installed by exact version.
func (r *RustInstaller) ListRemote(ctx context.Context) ([]string, error) {
	v, err := r.Resolve(ctx, "stable")
	if err != nil {
		return nil, err
	}
	return []string{v}, nil
}

// bootstrapRustup installs rustup into <root>/cargo/bin once, from a
// checksum-verified rustup-init, without touching shell profiles.
func bootstrapRustup(ctx context.Context, root, host string) error {
	if _, err := os.Stat(rustupPath(root)); err == nil {
		return nil
	}
	url := rustupDistURL + "/" + host + "/rustup-init"
	sum, err := fetchSmall(ctx, url+".sha256")
	if err != nil {
		return fmt.Errorf("fetch rustup-init checksum: %w", err)
	}
	fields := strings.Fields(string(sum))
	if len(fields) == 0 {
		return errors.New("empty rustup-init checksum")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	initPath, err := downloadVerifiedTo(ctx, url, fields[0], root)
	if err != nil {
		return err
	}
	defer os.Remove(initPath)
	if err := os.Chmod(initPath, 0o755); err != nil {
		return err
	}
	args := []string{"-y", "--no-modify-path", "--default-toolchain", "none", "--profile", "minimal"}
	if err := runCmd(ctx, initPath, args, rustEnv(root)); err != nil {
		return fmt.Errorf("rustup-init: %w", err)
	}
	if _, err := os.Stat(rustupPath(root)); err != nil {
		return fmt.Errorf("rustup-init did not install %s", rustupPath(root))
	}
	return nil
}

// Install installs toolchain req.Version with the private rustup and links
// req.Dest/bin to the toolchain's bin directory.
func (r *RustInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	host, err := rustHostTriple(hostOS, hostArch)
	if err != nil {
		return err
	}
	if err := bootstrapRustup(ctx, req.Root, host); err != nil {
		return err
	}
	args := []string{"toolchain", "install", req.Version, "--profile", "minimal", "--no-self-update"}
	if err := runCmd(ctx, rustupPath(req.Root), args, rustEnv(req.Root)); err != nil {
		return fmt.Errorf("rustup toolchain install %s: %w", req.Version, err)
	}
	bin := filepath.Join(req.Root, "rustup", "toolchains", req.Version+"-"+host, "bin")
	if fi, err := os.Stat(bin); err != nil || !fi.IsDir() {
		return fmt.Errorf("rustup installed no toolchain at %s", bin)
	}
	return os.Symlink(bin, filepath.Join(req.Dest, "bin"))
}

// Remove uninstalls the toolchain from the private rustup.
func (r *RustInstaller) Remove(ctx context.Context, version, _, root string) error {
	if _, err := os.Stat(rustupPath(root)); err != nil {
		return nil // nothing to uninstall
	}
	return runCmd(ctx, rustupPath(root), []string{"toolchain", "uninstall", version}, rustEnv(root))
}

// BinaryPaths returns the Rust binaries.
func (r *RustInstaller) BinaryPaths() []string { return []string{"bin/rustc", "bin/cargo"} }

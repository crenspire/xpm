package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

const rustStable = "manifest-version = \"2\"\n[pkg.cargo]\nversion = \"0.100.0 (abc 2026-09-20)\"\n[pkg.rust]\nversion = \"1.99.0 (b940084d7 2026-09-28)\"\n"

func TestParseRustChannelVersion(t *testing.T) {
	if v, err := parseRustChannelVersion(rustStable); err != nil || v != "1.99.0" {
		t.Fatalf("got %q, %v", v, err)
	}
	if _, err := parseRustChannelVersion("[pkg.cargo]\nversion = \"1.0.0 (x)\"\n"); err == nil {
		t.Fatal("manifest without [pkg.rust] accepted")
	}
}

func TestRustResolve(t *testing.T) {
	url, _ := newServer(t, map[string]route{
		"/channel-rust-stable.toml": {body: []byte(rustStable)},
		"/channel-rust-1.80.toml":   {body: []byte("[pkg.rust]\nversion = \"1.80.1 (3f5fd8dd4 2024-08-06)\"\n")},
	})
	setVar(t, &rustDistURL, url)
	r := &RustInstaller{}
	ctx := context.Background()
	for spec, want := range map[string]string{"stable": "1.99.0", "latest": "1.99.0", "1.80": "1.80.1", "1.75.0": "1.75.0"} {
		if v, err := env.ResolveSpec(ctx, r, spec); err != nil || v != want {
			t.Errorf("%s: %q, %v; want %s", spec, v, err, want)
		}
	}
	for _, spec := range []string{"nightly", "beta"} {
		_, err := r.Resolve(ctx, spec)
		if err == nil || err.Error() != "rust channels other than stable are not supported; use an exact version or stable" {
			t.Errorf("%s: %v", spec, err)
		}
	}
	if _, err := r.Resolve(ctx, "1.81"); err == nil {
		t.Error("missing channel accepted")
	}
}

type cmdCall struct {
	name string
	args []string
	env  []string
}

func TestRustInstallBootstrapsPrivateRustup(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	host := "x86_64-unknown-linux-gnu"
	initBin := []byte("rustup-init-binary")
	url, _ := newServer(t, map[string]route{
		"/" + host + "/rustup-init":        {body: initBin},
		"/" + host + "/rustup-init.sha256": {body: []byte(shaBytes(initBin) + " *./rustup-init\n")},
	})
	setVar(t, &rustupDistURL, url)

	var calls []cmdCall
	setVar(t, &runCmd, func(_ context.Context, name string, args, extra []string) error {
		calls = append(calls, cmdCall{name, args, extra})
		root := filepath.Dir(strings.TrimPrefix(extra[1], "CARGO_HOME="))
		if filepath.Base(name) != "rustup" { // rustup-init: lay down cargo/bin/rustup
			if err := os.MkdirAll(filepath.Join(root, "cargo", "bin"), 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(root, "cargo", "bin", "rustup"), []byte("rustup"), 0o755)
		}
		if args[1] != "install" {
			return nil
		}
		bin := filepath.Join(root, "rustup", "toolchains", args[2]+"-"+host, "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			return err
		}
		for _, b := range []string{"rustc", "cargo"} {
			if err := os.WriteFile(filepath.Join(bin, b), []byte(b), 0o755); err != nil {
				return err
			}
		}
		return nil
	})

	req := installReq(t, "1.99.0")
	if err := (&RustInstaller{}).Install(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %+v", calls)
	}
	if strings.Join(calls[0].args, " ") != "-y --no-modify-path --default-toolchain none --profile minimal" {
		t.Fatalf("rustup-init args = %v", calls[0].args)
	}
	wantEnv := []string{"RUSTUP_HOME=" + filepath.Join(req.Root, "rustup"), "CARGO_HOME=" + filepath.Join(req.Root, "cargo"), "RUSTUP_INIT_SKIP_PATH_CHECK=yes"}
	if strings.Join(calls[0].env, "|") != strings.Join(wantEnv, "|") {
		t.Fatalf("env = %v", calls[0].env)
	}
	if calls[1].name != filepath.Join(req.Root, "cargo", "bin", "rustup") ||
		strings.Join(calls[1].args, " ") != "toolchain install 1.99.0 --profile minimal --no-self-update" {
		t.Fatalf("toolchain call = %+v", calls[1])
	}
	if got, _ := readFile(req.Dest, "bin/rustc"); got != "rustc" {
		t.Fatalf("bin/rustc = %q", got)
	}

	// Second toolchain: rustup is already there, no second bootstrap.
	calls = nil
	req2 := env.InstallRequest{Version: "1.80.1", Dest: t.TempDir(), Root: req.Root}
	if err := (&RustInstaller{}).Install(context.Background(), req2); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].args[0] != "toolchain" {
		t.Fatalf("calls = %+v", calls)
	}

	calls = nil
	if err := (&RustInstaller{}).Remove(context.Background(), "1.80.1", req2.Dest, req.Root); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || strings.Join(calls[0].args, " ") != "toolchain uninstall 1.80.1" {
		t.Fatalf("remove = %+v", calls)
	}
}

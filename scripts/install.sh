#!/bin/sh
# Install xpm from a GitHub release, verifying the archive against the
# release's checksums.txt before anything is installed.
#
#   curl -fsSL https://crenspire.github.io/xpm/install.sh | sh
#   curl -fsSL https://crenspire.github.io/xpm/install.sh | sh -s -- --version v0.1.0 --dir "$HOME/bin"
#
# If no release has been published yet (or with --from-source), it builds
# xpm with `go install` instead, which needs Go 1.22 or newer.
#
# Run `sh install.sh --help` for all options.
set -eu

usage() {
	cat <<'EOF'
Usage: install.sh [--version vX.Y.Z] [--dir DIR] [--from-source] [--help]

Downloads the xpm release archive for this OS and CPU, checks its SHA-256
against the release's checksums.txt and installs the xpm binary. When no
release has been published yet, it builds xpm from source with `go install`
(Go 1.22 or newer required).

Options (each can also be set with an environment variable):
  --version vX.Y.Z  XPM_VERSION       Release to install (default: latest)
  --dir DIR         XPM_INSTALL_DIR   Install directory (default: /usr/local/bin
                                      if writable, else $HOME/.local/bin)
  --from-source     XPM_FROM_SOURCE=1 Build with `go install` instead of
                                      downloading a release archive
                    XPM_RELEASES_URL  Releases page (default:
                                      https://github.com/crenspire/xpm/releases)
                    XPM_DOWNLOAD_URL  Base download URL (default:
                                      $XPM_RELEASES_URL/download)
  -h, --help                          Show this help

Supported: Linux and macOS on amd64 and arm64. On Windows, download the zip
from https://github.com/crenspire/xpm/releases.
EOF
}

err() {
	echo "error: $*" >&2
	exit 1
}

module=github.com/crenspire/xpm/cmd/xpm
version="${XPM_VERSION:-}"
dir="${XPM_INSTALL_DIR:-}"
from_source="${XPM_FROM_SOURCE:-}"
releases="${XPM_RELEASES_URL:-https://github.com/crenspire/xpm/releases}"
base="${XPM_DOWNLOAD_URL:-$releases/download}"

while [ $# -gt 0 ]; do
	case "$1" in
	--version)
		[ $# -ge 2 ] || err "--version needs a value"
		version="$2"
		shift 2
		;;
	--version=*)
		version="${1#--version=}"
		shift
		;;
	--dir)
		[ $# -ge 2 ] || err "--dir needs a value"
		dir="$2"
		shift 2
		;;
	--dir=*)
		dir="${1#--dir=}"
		shift
		;;
	--from-source)
		from_source=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage >&2
		err "unknown argument: $1"
		;;
	esac
done

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) err "unsupported OS $(uname -s); on Windows download the zip from https://github.com/crenspire/xpm/releases" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) err "unsupported architecture $(uname -m)" ;;
esac

if command -v curl >/dev/null 2>&1; then
	download() { curl -fsSL -o "$2" "$1"; }
	fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -qO "$2" "$1"; }
	fetch() { wget -qO- "$1"; }
else
	err "need curl or wget to download xpm"
fi

if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
	err "need sha256sum or shasum to verify the download"
fi

if [ -z "$dir" ]; then
	if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
		dir=/usr/local/bin
	else
		dir="$HOME/.local/bin"
	fi
fi

path_note() {
	case ":${PATH}:" in
	*":$1:"*) ;;
	*) echo "Note: $1 is not in your PATH; add it, e.g. export PATH=\"$1:\$PATH\"" ;;
	esac
}

# latest_tag prints the newest release tag, or nothing when there is none.
# It follows the releases/latest redirect (no API rate limit) and falls back
# to the GitHub API when only wget is available.
latest_tag() {
	if command -v curl >/dev/null 2>&1; then
		final=$(curl -fsSL -o /dev/null -w '%{url_effective}' "$releases/latest" 2>/dev/null) || final=""
		case "$final" in
		*/tag/*) echo "${final##*/tag/}" ;;
		esac
	else
		fetch https://api.github.com/repos/crenspire/xpm/releases/latest 2>/dev/null |
			sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1
	fi
}

# install_from_source builds xpm with `go install` into $dir.
install_from_source() {
	command -v go >/dev/null 2>&1 ||
		err "$1, and Go is not installed. Install Go 1.22+ (https://go.dev/dl/) and re-run, or wait for a release"
	gover=$(go env GOVERSION 2>/dev/null || true)
	minor=$(echo "$gover" | sed -n 's/^go1\.\([0-9][0-9]*\).*/\1/p')
	if [ -z "$minor" ] || [ "$minor" -lt 22 ]; then
		err "$1, and building from source needs Go 1.22 or newer (found ${gover:-unknown})"
	fi
	ref="${version:-latest}"
	echo "$1; building xpm from source with $gover (go install $module@$ref)..."
	mkdir -p "$dir"
	GOBIN="$dir" go install "$module@$ref" || err "go install $module@$ref failed"
	echo "xpm ($ref, built from source) installed to $dir/xpm"
	path_note "$dir"
	exit 0
}

if [ -n "$from_source" ]; then
	install_from_source "Building from source as requested"
fi

if [ -z "$version" ]; then
	version=$(latest_tag)
	[ -n "$version" ] || install_from_source "No xpm release has been published yet"
fi
case "$version" in
v*) tag="$version" ;;
*) tag="v$version" ;;
esac
ver="${tag#v}"

archive="xpm_${ver}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "Downloading $archive ($tag)..."
download "$base/$tag/$archive" "$tmp/$archive" || err "download failed: $base/$tag/$archive"
download "$base/$tag/checksums.txt" "$tmp/checksums.txt" || err "download failed: $base/$tag/checksums.txt"

expected=$(awk -v f="$archive" '$2 == f {print $1; exit}' "$tmp/checksums.txt")
[ -n "$expected" ] || err "checksums.txt has no entry for $archive"
actual=$(sha256 "$tmp/$archive")
if [ "$expected" != "$actual" ]; then
	err "checksum mismatch for $archive (expected $expected, got $actual); not installing"
fi

mkdir -p "$tmp/extract"
tar -xzf "$tmp/$archive" -C "$tmp/extract"
[ -f "$tmp/extract/xpm" ] || err "$archive does not contain xpm"

# Stage next to the target and rename, so an existing xpm is replaced
# atomically and a failed copy never leaves a partial binary behind.
mkdir -p "$dir"
staged="$dir/.xpm.$$"
trap 'rm -rf "$tmp"; rm -f "$staged"' EXIT
if ! install -m 0755 "$tmp/extract/xpm" "$staged" 2>/dev/null; then
	cp "$tmp/extract/xpm" "$staged" || err "could not write $staged"
	chmod 0755 "$staged" || err "could not chmod $staged"
fi
mv -f "$staged" "$dir/xpm" || err "could not move $staged to $dir/xpm"

echo "xpm $tag installed to $dir/xpm"
path_note "$dir"

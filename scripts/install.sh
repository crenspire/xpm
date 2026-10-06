#!/bin/sh
# Install xpm from a GitHub release, verifying the archive against the
# release's checksums.txt before anything is installed.
#
#   curl -fsSL https://raw.githubusercontent.com/crenspire/xpm/main/scripts/install.sh | sh
#   sh install.sh --version v0.1.0 --dir "$HOME/bin"
#
# Run `sh install.sh --help` for all options.
set -eu

usage() {
	cat <<'EOF'
Usage: install.sh [--version vX.Y.Z] [--dir DIR] [--help]

Downloads the xpm release archive for this OS and CPU, checks its SHA-256
against the release's checksums.txt and installs the xpm binary.

Options (each can also be set with an environment variable):
  --version vX.Y.Z  XPM_VERSION       Release to install (default: latest)
  --dir DIR         XPM_INSTALL_DIR   Install directory (default: /usr/local/bin
                                      if writable, else $HOME/.local/bin)
                    XPM_DOWNLOAD_URL  Base download URL (default:
                                      https://github.com/crenspire/xpm/releases/download)
  -h, --help                          Show this help

Supported: Linux and macOS on amd64 and arm64. On Windows, download the zip
from https://github.com/crenspire/xpm/releases.
EOF
}

err() {
	echo "error: $*" >&2
	exit 1
}

version="${XPM_VERSION:-}"
dir="${XPM_INSTALL_DIR:-}"
base="${XPM_DOWNLOAD_URL:-https://github.com/crenspire/xpm/releases/download}"

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

if [ -z "$version" ]; then
	version=$(fetch https://api.github.com/repos/crenspire/xpm/releases/latest |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
	[ -n "$version" ] || err "could not determine the latest xpm release; set XPM_VERSION or pass --version"
fi
case "$version" in
v*) tag="$version" ;;
*) tag="v$version" ;;
esac
ver="${tag#v}"

if [ -z "$dir" ]; then
	if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
		dir=/usr/local/bin
	else
		dir="$HOME/.local/bin"
	fi
fi

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

mkdir -p "$dir"
if ! install -m 0755 "$tmp/extract/xpm" "$dir/xpm" 2>/dev/null; then
	cp "$tmp/extract/xpm" "$dir/xpm"
	chmod 0755 "$dir/xpm"
fi

echo "xpm $tag installed to $dir/xpm"
case ":${PATH}:" in
*":$dir:"*) ;;
*) echo "Note: $dir is not in your PATH; add it, e.g. export PATH=\"$dir:\$PATH\"" ;;
esac

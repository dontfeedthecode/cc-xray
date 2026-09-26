#!/bin/sh
# Install the latest ccxray release.
#   curl -fsSL https://raw.githubusercontent.com/dontfeedthecode/cc-xray/main/install.sh | sh
#
# CCXRAY_VERSION=vX.Y.Z     install a specific release
# CCXRAY_INSTALL_DIR=<dir>  install somewhere other than ~/.local/bin
set -eu

REPO="dontfeedthecode/cc-xray"
BIN="ccxray"

fail() { echo "ccxray: $*" >&2; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail "unsupported architecture: $arch" ;;
esac
case "$os" in
  darwin|linux) ;;
  # Git Bash, and Claude Code's ! prefix on Windows, land here
  mingw*|msys*|cygwin*) fail "on Windows, install from PowerShell instead:
  irm https://raw.githubusercontent.com/$REPO/main/install.ps1 | iex
  or from here: powershell -c \"irm https://raw.githubusercontent.com/$REPO/main/install.ps1 | iex\"" ;;
  *) fail "unsupported OS: $os (macOS, Linux and Windows only)" ;;
esac

tag=${CCXRAY_VERSION:-}
if [ -z "$tag" ]; then
  tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
fi
[ -n "$tag" ] || fail "could not determine the latest release.
  Set CCXRAY_VERSION=vX.Y.Z, or download from https://github.com/$REPO/releases"

ver=${tag#v}
archive="${BIN}_${ver}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "ccxray: downloading $tag ($os/$arch)"
curl -fsSL "$base/$archive" -o "$tmp/$archive" || fail "download failed: $base/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "download failed: $base/checksums.txt"

want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
else
  got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
fi
[ -n "$want" ] && [ "$want" = "$got" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" "$BIN"

dest=${CCXRAY_INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$dest"
mv "$tmp/$BIN" "$dest/$BIN" || fail "cannot write to $dest; set CCXRAY_INSTALL_DIR"
chmod +x "$dest/$BIN"

echo "ccxray: installed $tag to $dest/$BIN"
case ":$PATH:" in
  *":$dest:"*)
    echo "ccxray: run 'ccxray' in a second terminal, in the directory Claude Code is working in." ;;
  *)
    case "${SHELL##*/}" in
      zsh) rc="~/.zshrc" ;;
      bash) rc="~/.bashrc" ;;
      fish) rc="" ;;
      *) rc="~/.profile" ;;
    esac
    echo "ccxray: $dest is not on your PATH. Add it with:"
    if [ -z "$rc" ]; then
      echo "  fish_add_path $dest"
    else
      echo "  echo 'export PATH=\"$dest:\$PATH\"' >> $rc"
    fi ;;
esac

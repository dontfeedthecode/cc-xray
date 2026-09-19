#!/bin/sh
# Install the latest ccxray release.
#   curl -fsSL https://raw.githubusercontent.com/dontfeedthecode/ccxray/main/install.sh | sh
set -eu

REPO="dontfeedthecode/ccxray"
BIN="ccxray"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "ccxray: unsupported architecture: $arch" >&2; exit 1 ;;
esac
case "$os" in
  darwin|linux) ;;
  *) echo "ccxray: unsupported OS: $os" >&2; exit 1 ;;
esac

tag=${CCXRAY_VERSION:-}
if [ -z "$tag" ]; then
  tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
fi
if [ -z "$tag" ]; then
  echo "ccxray: could not determine the latest release." >&2
  echo "  Set CCXRAY_VERSION=vX.Y.Z, or download from" >&2
  echo "  https://github.com/$REPO/releases" >&2
  exit 1
fi

ver=${tag#v}
url="https://github.com/$REPO/releases/download/$tag/${BIN}_${ver}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "ccxray: downloading $tag ($os/$arch)"
if ! curl -fsSL "$url" -o "$tmp/$BIN.tar.gz"; then
  echo "ccxray: download failed: $url" >&2
  exit 1
fi
tar -xzf "$tmp/$BIN.tar.gz" -C "$tmp"

# prefer a writable directory already on PATH; fall back to ~/.local/bin
dest=""
for d in "$HOME/.local/bin" /usr/local/bin; do
  case ":$PATH:" in *":$d:"*) [ -w "$d" ] && dest="$d" && break ;; esac
done
[ -n "$dest" ] || dest="$HOME/.local/bin"
mkdir -p "$dest"

if ! mv "$tmp/$BIN" "$dest/$BIN" 2>/dev/null; then
  echo "ccxray: installing to $dest needs elevated permissions"
  sudo mv "$tmp/$BIN" "$dest/$BIN"
fi
chmod +x "$dest/$BIN"

echo "ccxray: installed to $dest/$BIN"
case ":$PATH:" in
  *":$dest:"*) echo "ccxray: run 'ccxray' in the directory Claude Code is working in." ;;
  *) echo "ccxray: add $dest to your PATH:"
     echo "  echo 'export PATH=\"$dest:\$PATH\"' >> ~/.zshrc" ;;
esac

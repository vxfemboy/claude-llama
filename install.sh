#!/usr/bin/env bash
# claude-llama-mcp installer.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/vxfemboy/claude-llama/main/install.sh | sh
#
# Honors:
#   CLAUDE_LLAMA_BIN_DIR  install dir (default: $HOME/.local/bin)
#   CLAUDE_LLAMA_VERSION  release tag to install (default: latest)
#   CLAUDE_LLAMA_REPO     GitHub repo (default: vxfemboy/claude-llama)
#   CLAUDE_LLAMA_SKIP_INIT=1   skip running `claude-llama-mcp init` at the end
set -euo pipefail

REPO="${CLAUDE_LLAMA_REPO:-vxfemboy/claude-llama}"
BIN_DIR="${CLAUDE_LLAMA_BIN_DIR:-$HOME/.local/bin}"
VERSION="${CLAUDE_LLAMA_VERSION:-latest}"

say() { printf "claude-llama-installer: %s\n" "$*"; }
die() { printf "claude-llama-installer: error: %s\n" "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"; }
need curl
need tar
need uname

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac
case "$os" in
  linux|darwin) ;;
  *) die "unsupported OS: $os" ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  resp=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null) \
    || die "no published releases found for ${REPO}. Cut one by pushing a version tag (e.g. \`git tag v0.1.0 && git push origin v0.1.0\`), then re-run. See https://github.com/${REPO}/releases"
  VERSION=$(printf '%s' "$resp" | grep '"tag_name"' | head -n1 | sed -E 's/.*"([^"]+)".*/\1/')
  [[ -n "$VERSION" ]] || die "could not resolve latest version from ${REPO}"
fi
ver_noV="${VERSION#v}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

archive="claude-llama-mcp_${ver_noV}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${VERSION}/${archive}"
sums_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

say "downloading $archive"
curl -fsSL "$url" -o "$tmp/$archive" \
  || die "could not download $archive from release $VERSION. Check that release exists at https://github.com/${REPO}/releases/tag/${VERSION}"
curl -fsSL "$sums_url" -o "$tmp/checksums.txt" \
  || die "could not download checksums.txt for release $VERSION"

say "verifying checksum"
(cd "$tmp" && grep " $archive\$" checksums.txt | sha256sum -c -) >/dev/null || die "checksum failed"

mkdir -p "$BIN_DIR"
tar -xzf "$tmp/$archive" -C "$tmp"
install -m 0755 "$tmp/claude-llama-mcp" "$BIN_DIR/claude-llama-mcp"
say "installed $BIN_DIR/claude-llama-mcp"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) say "note: $BIN_DIR is not on \$PATH — add it to your shell rc" ;;
esac

if [[ "${CLAUDE_LLAMA_SKIP_INIT:-0}" != "1" ]]; then
  say "running \`claude-llama-mcp init\` (re-run with --force later to change)"
  "$BIN_DIR/claude-llama-mcp" init || say "init exited non-zero; re-run manually when ready"
fi

cat <<'EOF'

next:
  1. start llama.cpp on the URL you chose (default http://localhost:8080)
  2. run: claude-llama-mcp doctor
  3. add to your project's .mcp.json:
       { "mcpServers": { "claude-llama": { "command": "claude-llama-mcp" } } }
EOF

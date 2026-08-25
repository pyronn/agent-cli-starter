#!/bin/sh
set -eu

cli_name="agentctl"
repository_url="https://github.com/pyronn/agent-cli-starter"
repository="${repository_url#https://github.com/}"
version="${AGENTCTL_VERSION:-${1:-latest}}"
install_dir="${AGENTCTL_INSTALL_DIR:-${2:-$HOME/.local/bin}}"

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"

case "$(uname -s)" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *) fail "unsupported operating system; use install.ps1 on Windows" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) fail "unsupported CPU architecture: $(uname -m)" ;;
esac

if [ "$version" = "latest" ]; then
  release_url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$repository/releases/latest")"
  version="${release_url##*/}"
fi

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) fail "invalid version '$version'; expected a tag such as v1.2.3" ;;
esac

archive="${cli_name}-${version}-${os}-${arch}.tar.gz"
binary="${cli_name}-${os}-${arch}"
base_url="https://github.com/${repository}/releases/download/${version}"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM

printf 'Downloading %s %s for %s/%s...\n' "$cli_name" "$version" "$os" "$arch"
curl -fL --retry 3 --output "$temp_dir/$archive" "$base_url/$archive"
curl -fL --retry 3 --output "$temp_dir/SHA256SUMS" "$base_url/SHA256SUMS"

expected="$(awk -v file="$archive" '$2 == file { print $1 }' "$temp_dir/SHA256SUMS")"
[ -n "$expected" ] || fail "checksum for $archive is missing"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temp_dir/$archive" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$temp_dir/$archive" | awk '{ print $1 }')"
else
  fail "sha256sum or shasum is required to verify the download"
fi
[ "$actual" = "$expected" ] || fail "SHA256 checksum mismatch for $archive"

tar -xzf "$temp_dir/$archive" -C "$temp_dir"
[ -f "$temp_dir/$binary" ] || fail "archive does not contain $binary"
mkdir -p "$install_dir"
install -m 0755 "$temp_dir/$binary" "$install_dir/$cli_name"

printf 'Installed %s %s to %s/%s\n' "$cli_name" "$version" "$install_dir" "$cli_name"
case ":$PATH:" in
  *":$install_dir:"*) "$install_dir/$cli_name" version ;;
  *)
    printf 'Add this directory to PATH, then reopen your terminal:\n'
    printf '  export PATH="%s:$PATH"\n' "$install_dir"
    "$install_dir/$cli_name" version
    ;;
esac

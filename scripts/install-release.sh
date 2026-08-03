#!/usr/bin/env sh
set -eu

release_version="${AGENT_TELEMETRY_RELEASE_VERSION:-latest}"
github_repo="${AGENT_TELEMETRY_GITHUB_REPO:-GuanceCloud/agent-telemetry}"

usage() {
  cat <<'EOF'
install-release.sh [--release-version <version|latest>] [--github-repo <owner/repo>] [agent-telemetry install args...]

Examples:
  install-release.sh --type gtrace --endpoint https://llm-openway.guance.com --x-token '<token>' --enable
  install-release.sh --release-version v0.3.0-rc.4 codex --type otlp --endpoint http://127.0.0.1:4318 --enable
EOF
}

fail() {
  echo "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "Missing required command: $1"
}

download() {
  url="$1"
  destination="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$destination"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$destination" "$url"
    return
  fi
  fail "Missing required command: curl or wget"
}

sha256_file() {
  file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return
  fi
  fail "Missing required command: sha256sum or shasum"
}

verify_checksum() {
  asset_name="$1"
  sums_file="$2"
  archive_file="$3"
  expected="$(awk -v name="$asset_name" '$2 == name { print $1; exit }' "$sums_file")"
  [ -n "$expected" ] || fail "Missing checksum entry for $asset_name"
  actual="$(sha256_file "$archive_file")"
  [ "$expected" = "$actual" ] || fail "Checksum mismatch for $asset_name"
}

platform_name() {
  os_name="$(uname -s)"
  arch_name="$(uname -m)"
  case "$os_name" in
    Linux) goos="linux" ;;
    Darwin) goos="darwin" ;;
    *) fail "Unsupported operating system: $os_name" ;;
  esac
  case "$arch_name" in
    x86_64|amd64) goarch="amd64" ;;
    arm64|aarch64) goarch="arm64" ;;
    *) fail "Unsupported architecture: $arch_name" ;;
  esac
  printf '%s-%s' "$goos" "$goarch"
}

temp_dir() {
  if command -v mktemp >/dev/null 2>&1; then
    mktemp -d 2>/dev/null && return
    mktemp -d -t agent-telemetry-install && return
  fi
  fail "Missing required command: mktemp"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --release-version)
      [ $# -ge 2 ] || fail "Missing value for --release-version"
      release_version="$2"
      shift 2
      ;;
    --github-repo)
      [ $# -ge 2 ] || fail "Missing value for --github-repo"
      github_repo="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    *)
      break
      ;;
  esac
done

require_command tar
work_dir="$(temp_dir)"
trap 'rm -rf "$work_dir"' EXIT HUP INT TERM

platform="$(platform_name)"
archive_ext="tar.gz"
normalized_version="$(printf '%s' "$release_version" | sed 's/^v//')"
if [ "$normalized_version" = "latest" ]; then
  base_url="https://github.com/${github_repo}/releases/latest/download"
  archive_name="agent-telemetry-${platform}.${archive_ext}"
else
  tag="v${normalized_version}"
  base_url="https://github.com/${github_repo}/releases/download/${tag}"
  archive_name="agent-telemetry-v${normalized_version}-${platform}.${archive_ext}"
fi

sums_file="${work_dir}/SHA256SUMS"
archive_file="${work_dir}/${archive_name}"
download "${base_url}/SHA256SUMS" "$sums_file"
download "${base_url}/${archive_name}" "$archive_file"
verify_checksum "$archive_name" "$sums_file" "$archive_file"

extract_dir="${work_dir}/extract"
mkdir -p "$extract_dir"
tar -xzf "$archive_file" -C "$extract_dir"

installer="${extract_dir}/scripts/install.sh"
[ -f "$installer" ] || fail "Release archive is missing scripts/install.sh"
chmod +x "$installer"
exec "$installer" "$@"

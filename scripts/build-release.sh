#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
DIST_DIR="${DIST_DIR:-${REPO_ROOT}/dist}"
VERSION="$(tr -d '[:space:]' <"${REPO_ROOT}/VERSION")"
TARGETS=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

if [[ -z "${VERSION}" ]]; then
  echo "VERSION is empty" >&2
  exit 1
fi
if [[ -z "${DIST_DIR}" ]]; then
  echo "Unsafe DIST_DIR: ${DIST_DIR}" >&2
  exit 1
fi
mkdir -p "${DIST_DIR}"
DIST_DIR="$(cd "${DIST_DIR}" && pwd)"
if [[ "${DIST_DIR}" == "/" || "${DIST_DIR}" == "${REPO_ROOT}" || "${DIST_DIR}" == "${HOME}" ]]; then
  echo "Unsafe DIST_DIR: ${DIST_DIR}" >&2
  exit 1
fi
command -v go >/dev/null 2>&1 || {
  echo "Missing required command: go" >&2
  exit 1
}
command -v tar >/dev/null 2>&1 || {
  echo "Missing required command: tar" >&2
  exit 1
}

rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

for target in "${TARGETS[@]}"; do
  goos="${target%% *}"
  goarch="${target##* }"
  platform="${goos}-${goarch}"
  stage="${DIST_DIR}/stage-${platform}"
  binary_name="agent-telemetry"
  if [[ "${goos}" == "windows" ]]; then
    binary_name="agent-telemetry.exe"
  fi

  mkdir -p \
    "${stage}/bin" \
    "${stage}/scripts"
  (
    cd "${REPO_ROOT}"
    env CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
      go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X github.com/GuanceCloud/agent-telemetry/internal/adapters/claude/buildinfo.Version=${VERSION} -X github.com/GuanceCloud/agent-telemetry/internal/adapters/codex/buildinfo.Version=${VERSION}" \
      -o "${stage}/bin/${binary_name}" ./cmd/agent-telemetry
  )
  cp "${REPO_ROOT}/plugin.json" "${stage}/plugin.json"
  cp "${REPO_ROOT}/README.md" "${stage}/README.md"
  cp "${REPO_ROOT}/VERSION" "${stage}/VERSION"
  cp "${REPO_ROOT}/scripts/install.sh" "${stage}/scripts/install.sh"
  cp "${REPO_ROOT}/scripts/install.ps1" "${stage}/scripts/install.ps1"

  if [[ "${goos}" == "windows" ]]; then
    command -v zip >/dev/null 2>&1 || {
      echo "Missing required command: zip" >&2
      exit 1
    }
    (
      cd "${stage}"
      zip -qr "${DIST_DIR}/agent-telemetry-v${VERSION}-${platform}.zip" .
    )
  else
    tar -czf "${DIST_DIR}/agent-telemetry-v${VERSION}-${platform}.tar.gz" -C "${stage}" .
  fi
  rm -rf "${stage}"
done

(
  cd "${DIST_DIR}"
  sha256sum agent-telemetry-v* >SHA256SUMS
)

echo "Built release assets in ${DIST_DIR}"

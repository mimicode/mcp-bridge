#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-mcp-bridge}"
COMMIT="${COMMIT:-$(git -C "${ROOT_DIR}" rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
OUT_DIR="${OUT_DIR:-${ROOT_DIR}/release}"
RUN_TESTS="${RUN_TESTS:-1}"
TARGETS="${TARGETS:-darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

resolve_version() {
  if [[ -n "${VERSION:-}" ]]; then
    echo "${VERSION}"
    return
  fi

  if [[ "${GITHUB_REF_TYPE:-}" == "tag" && -n "${GITHUB_REF_NAME:-}" ]]; then
    echo "${GITHUB_REF_NAME}"
    return
  fi

  if git -C "${ROOT_DIR}" describe --tags --exact-match >/dev/null 2>&1; then
    git -C "${ROOT_DIR}" describe --tags --exact-match
    return
  fi

  echo "dev"
}

build_frontend() {
  require_command npm

  echo "==> build admin ui"
  (
    cd "${ROOT_DIR}/web"
    npm ci
    npm run build
  )
}

write_release_config() {
  local file_path="$1"
  cat >"${file_path}" <<'EOF'
{
  "mcpServers": {}
}
EOF
}

run_tests() {
  if [[ "${RUN_TESTS}" != "1" ]]; then
    return
  fi

  echo "==> run go test"
  (
    cd "${ROOT_DIR}"
    go test ./...
  )
}

package_target() {
  local goos="$1"
  local goarch="$2"
  local dist_name="${APP_NAME}_${VERSION}_${goos}_${goarch}"
  local work_dir="${OUT_DIR}/${dist_name}"
  local binary_name="${APP_NAME}"

  if [[ "${goos}" == "windows" ]]; then
    binary_name="${binary_name}.exe"
  fi

  mkdir -p "${work_dir}"

  echo "==> build ${goos}/${goarch}"
  (
    cd "${ROOT_DIR}"
    CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
      go build -trimpath \
      -ldflags="-s -w -X github.com/mimicode/mcp_bridge/internal/buildinfo.Version=${VERSION} -X github.com/mimicode/mcp_bridge/internal/buildinfo.Commit=${COMMIT} -X github.com/mimicode/mcp_bridge/internal/buildinfo.BuildTime=${BUILD_TIME}" \
      -o "${work_dir}/${binary_name}" ./cmd/mcp-bridge
  )

  cp "${ROOT_DIR}/README.md" "${work_dir}/README.md"
  write_release_config "${work_dir}/config.json"

  if [[ "${goos}" == "windows" ]]; then
    (
      cd "${OUT_DIR}"
      zip -qr "${dist_name}.zip" "${dist_name}"
    )
  else
    tar -C "${OUT_DIR}" -czf "${OUT_DIR}/${dist_name}.tar.gz" "${dist_name}"
  fi

  rm -rf "${work_dir}"
}

main() {
  require_command go
  require_command tar
  require_command zip

  VERSION="$(resolve_version)"
  export VERSION

  rm -rf "${OUT_DIR}"
  mkdir -p "${OUT_DIR}"

  build_frontend
  run_tests

  for target in ${TARGETS}; do
    package_target "${target%/*}" "${target#*/}"
  done

  echo "==> done"
  echo "artifacts:"
  ls -1 "${OUT_DIR}"
}

main "$@"

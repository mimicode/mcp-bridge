#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_PATH="${CONFIG_PATH:-${ROOT_DIR}/config.json}"
LISTEN_ADDR="${LISTEN_ADDR:-:8082}"
BASE_PATH="${BASE_PATH:-/mcp}"
SKIP_BUILD="${SKIP_BUILD:-0}"
SKIP_INSTALL="${SKIP_INSTALL:-0}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

print_help() {
  cat <<EOF
Usage: ./scripts/dev.sh [options]

Options:
  --config <path>      Path to config json
  --listen <addr>      HTTP listen address
  --base-path <path>   Base path for auto-generated MCP routes
  --skip-build         Skip frontend build
  --skip-install       Skip automatic npm install when node_modules is missing
  -h, --help           Show this help message

Environment overrides:
  CONFIG_PATH
  LISTEN_ADDR
  BASE_PATH
  SKIP_BUILD=1
  SKIP_INSTALL=1
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config)
        CONFIG_PATH="$2"
        shift 2
        ;;
      --listen)
        LISTEN_ADDR="$2"
        shift 2
        ;;
      --base-path)
        BASE_PATH="$2"
        shift 2
        ;;
      --skip-build)
        SKIP_BUILD=1
        shift
        ;;
      --skip-install)
        SKIP_INSTALL=1
        shift
        ;;
      -h|--help)
        print_help
        exit 0
        ;;
      *)
        echo "unknown option: $1" >&2
        echo >&2
        print_help >&2
        exit 1
        ;;
    esac
  done
}

ensure_frontend_deps() {
  if [[ "${SKIP_INSTALL}" == "1" ]]; then
    return
  fi

  if [[ ! -d "${ROOT_DIR}/web/node_modules" ]]; then
    echo "==> install web dependencies"
    (
      cd "${ROOT_DIR}/web"
      npm install
    )
  fi
}

build_frontend() {
  if [[ "${SKIP_BUILD}" == "1" ]]; then
    echo "==> skip admin ui build"
    return
  fi

  ensure_frontend_deps

  echo "==> build admin ui"
  (
    cd "${ROOT_DIR}/web"
    npm run build
  )
}

main() {
  parse_args "$@"
  require_command npm
  require_command go

  build_frontend

  echo "==> start go server"
  (
    cd "${ROOT_DIR}"
    exec go run ./cmd/mcp-bridge \
      -config "${CONFIG_PATH}" \
      -listen "${LISTEN_ADDR}" \
      -base-path "${BASE_PATH}"
  )
}

main "$@"

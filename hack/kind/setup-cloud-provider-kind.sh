#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CLOUD_PROVIDER_KIND_BIN=$(go tool -modfile="${ROOT_DIR}/tools/go.mod" -n cloud-provider-kind)
CLOUD_PROVIDER_KIND_ARGS=(--gateway-channel=disabled)

cloud_provider_kind_running() {
  pgrep -x cloud-provider-kind >/dev/null
}

start_cloud_provider_kind() {
  if cloud_provider_kind_running; then
    echo "cloud-provider-kind already running, skipping startup"
    return
  fi

  echo "starting cloud-provider-kind with sudo"
  sudo -b "${CLOUD_PROVIDER_KIND_BIN}" "${CLOUD_PROVIDER_KIND_ARGS[@]}"
}

start_cloud_provider_kind

#!/usr/bin/env bash

set -euo pipefail

# Usage: install-gateway-api-crds.sh <version-or-commit> [channel]
version="${1:?Gateway API version or commit is required}"
channel="${2:-experimental}"

# Release bundles avoid remote kustomize's clone timeout. Keep kustomize for
# commit refs used when testing unreleased Gateway API changes.
if [[ $version =~ ^v[0-9] ]]; then
  manifest="$(mktemp "${TMPDIR:-/tmp}/gateway-api-crds.XXXXXX")"
  trap 'rm -f "$manifest"' EXIT

  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  "$script_dir/download-github-release-asset.sh" kubernetes-sigs/gateway-api "$version" "$channel-install.yaml" "$manifest"
  kubectl apply --server-side -f "$manifest"
elif [[ $channel == "standard" ]]; then
  kubectl apply --server-side --kustomize "https://github.com/kubernetes-sigs/gateway-api/config/crd?ref=$version"
else
  kubectl apply --server-side --kustomize "https://github.com/kubernetes-sigs/gateway-api/config/crd/$channel?ref=$version"
fi

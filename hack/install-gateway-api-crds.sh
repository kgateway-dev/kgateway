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

  # The web /releases/download/ endpoint can 504 before redirecting to the
  # asset, even while the asset API works. Bypass that endpoint entirely.
  # https://github.com/orgs/community/discussions/212407
  curl_args=(--fail --location --silent --show-error --connect-timeout 10 --max-time 30)
  # CI supplies a token to avoid the unauthenticated API rate limit. Public
  # release assets also work without a token for local setup.
  if [[ -n ${GITHUB_TOKEN:-} ]]; then
    curl_args+=(--header "Authorization: Bearer $GITHUB_TOKEN")
  fi
  if ! asset=$(curl "${curl_args[@]}" --header 'Accept: application/vnd.github+json' \
    "https://api.github.com/repos/kubernetes-sigs/gateway-api/releases/tags/$version" \
    | jq -ce --arg name "$channel-install.yaml" '.assets[] | select(.name == $name)'); then
    echo "Unable to resolve Gateway API $version release asset $channel-install.yaml" >&2
    exit 1
  fi
  asset_id=$(jq -er '.id' <<< "$asset")
  digest=$(jq -r '.digest // empty' <<< "$asset")
  # Download fully before applying so failed transfers never reach kubectl.
  curl "${curl_args[@]}" --header 'Accept: application/octet-stream' \
    --output "$manifest" \
    "https://api.github.com/repos/kubernetes-sigs/gateway-api/releases/assets/$asset_id"
  if [[ -z $digest ]]; then
    # Older release assets (for example v1.2.1) have no digest in GitHub's API.
    echo "Warning: Gateway API $version asset $channel-install.yaml has no published SHA-256 digest" >&2
  else
    if [[ ! $digest =~ ^sha256:[0-9a-f]{64}$ ]]; then
      echo "Invalid SHA-256 digest for Gateway API $version asset $channel-install.yaml: $digest" >&2
      exit 1
    fi
    if command -v sha256sum >/dev/null 2>&1; then
      actual_digest="sha256:$(sha256sum < "$manifest" | awk '{print $1}')"
    else
      actual_digest="sha256:$(shasum -a 256 < "$manifest" | awk '{print $1}')"
    fi
    if [[ $actual_digest != "$digest" ]]; then
      echo "SHA-256 mismatch for Gateway API $version asset $channel-install.yaml: expected $digest, got $actual_digest" >&2
      exit 1
    fi
  fi
  kubectl apply --server-side -f "$manifest"
elif [[ $channel == "standard" ]]; then
  kubectl apply --server-side --kustomize "https://github.com/kubernetes-sigs/gateway-api/config/crd?ref=$version"
else
  kubectl apply --server-side --kustomize "https://github.com/kubernetes-sigs/gateway-api/config/crd/$channel?ref=$version"
fi

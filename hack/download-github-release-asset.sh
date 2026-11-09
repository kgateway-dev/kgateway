#!/usr/bin/env bash

set -euo pipefail

# Usage: download-github-release-asset.sh <owner/repo> <tag> <asset-name> <destination>
repository="${1:?GitHub repository is required}"
version="${2:?Release tag is required}"
asset_name="${3:?Release asset name is required}"
destination="${4:?Download destination is required}"

# Publish only a completed, verified download; leave any existing destination
# untouched on failure. Creating the temporary file beside it keeps mv atomic.
manifest="$(mktemp "${destination}.XXXXXX")"
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
  "https://api.github.com/repos/$repository/releases/tags/$version" \
  | jq -ce --arg name "$asset_name" '.assets[] | select(.name == $name)'); then
  echo "Unable to resolve $repository $version release asset $asset_name" >&2
  exit 1
fi
asset_id=$(jq -er '.id' <<< "$asset")
digest=$(jq -r '.digest // empty' <<< "$asset")
# Download fully and verify before publishing the destination.
curl "${curl_args[@]}" --header 'Accept: application/octet-stream' \
  --output "$manifest" \
  "https://api.github.com/repos/$repository/releases/assets/$asset_id"
if [[ -z $digest ]]; then
  # Older release assets (for example v1.2.1) have no digest in GitHub's API.
  echo "Warning: $repository $version asset $asset_name has no published SHA-256 digest" >&2
else
  if [[ ! $digest =~ ^sha256:[0-9a-f]{64}$ ]]; then
    echo "Invalid SHA-256 digest for $repository $version asset $asset_name: $digest" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    actual_digest="sha256:$(sha256sum < "$manifest" | awk '{print $1}')"
  else
    actual_digest="sha256:$(shasum -a 256 < "$manifest" | awk '{print $1}')"
  fi
  if [[ $actual_digest != "$digest" ]]; then
    echo "SHA-256 mismatch for $repository $version asset $asset_name: expected $digest, got $actual_digest" >&2
    exit 1
  fi
fi

mv -f "$manifest" "$destination"

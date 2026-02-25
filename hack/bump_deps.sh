#!/usr/bin/env bash

set -o pipefail
set -o nounset
set -o errexit

# The base URLs for fetching CRDs
GATEWAY_CRD_BASE="https://raw.githubusercontent.com/kubernetes-sigs/gateway-api"

# The Gateway API channel (experimental or stable)
CONFORMANCE_CHANNEL="${CONFORMANCE_CHANNEL:-experimental}"

if [ $# -ne 2 ]; then
  echo "Usage: $0 gtw REF"
  echo "  REF can be a tag (e.g. v1.3.0) or a commit SHA."
  exit 2
fi

kind="$1"; shift
ref="$1"; shift

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Bumping $kind to ref: $ref ..."

# Check the current module version
if [ "$kind" != "gtw" ]; then
  echo "Unknown kind: $kind" >&2
  exit 1
fi
module="sigs.k8s.io/gateway-api"

current_version="$(go list -m -f '{{.Version}}' "$module" 2>/dev/null || true)"

# Resolve desired ref to a module version (pseudo-version if ref is non-tag)
resolved_version="$(
  go list -m -json "${module}@${ref}" 2>/dev/null \
    | sed -n 's/.*"Version": *"\([^"]*\)".*/\1/p'
)"

if [ -z "${resolved_version}" ]; then
  echo "ERROR: Could not resolve ${module}@${ref} to a module version."
  echo "       Verify the ref exists in the upstream repo."
  exit 1
fi

echo "Current version: ${current_version:-<none>}"
echo "Desired version (resolved): ${resolved_version}"

# If already at the desired version, skip go get/mod tidy but still refresh CRDs.
skip_get=0
if [ -n "$current_version" ] && [ "$current_version" = "$resolved_version" ]; then
  echo "Already at ${resolved_version} - skipping dependency bump, refreshing CRDs..."
  skip_get=1
fi

if [ $skip_get -eq 0 ]; then
  # Bump the module deps
  echo "Running: go get ${module}@${ref}"
  go get "${module}@${ref}"

  echo "Running: go mod tidy"
  go mod tidy
fi

update_make_var_line() {
  local file="$1"
  local var="$2"
  local val="$3"

  if [ ! -f "$file" ]; then
    echo "WARN: $file not found, skipping"
    return 0
  fi

  echo "Setting ${var} to '${val}' in ${file}"

  if grep -Eq "^[[:space:]]*${var}[[:space:]]*([?:]?=)" "$file"; then
    sed -i.bak -E \
      -e "s|^([[:space:]]*${var}[[:space:]]*[?:]?=)[[:space:]]*.*$|\1 ${val}|g" \
      "$file"
    rm -f "$file.bak"
  else
    printf '\n%s ?= %s\n' "$var" "$val" >> "$file"
  fi
}

update_nightly_gateway_api_matrix_versions() {
  local file="$1"
  local old="$2"
  local new="$3"

  if [ ! -f "$file" ]; then
    echo "WARN: $file not found, skipping"
    return 0
  fi

  # Escape dots for ERE (good enough for vX.Y.Z / -rc.N)
  local old_esc="${old//./\\.}"

  echo "Updating nightly-tests gateway-api matrix: ${old} -> ${new} in ${file}"
  sed -i.bak -E \
    -e "s|(version:[[:space:]]*')${old_esc}(')|\1${new}\2|g" \
    "$file"
  rm -f "$file.bak"
}

# Fetch and store the CRDs
crd_dir="$root/pkg/kgateway/crds"
mkdir -p "$crd_dir"
# Gateway API CRDs
# Use release assets when ref resolves to a non-pseudo version (tags incl. -rc.*),
# otherwise iterate files from the repo for pseudo-versions (timestamps/SHAs).
out_all="${crd_dir}/gateway-crds.yaml"

is_pseudo=0
if [[ "$resolved_version" =~ ([0-9]{14})-([0-9a-f]{7,40})$ ]]; then
  is_pseudo=1
fi

# Read current pinned conformance version from the Makefile (so we only replace “latest”)
current_conf="$(
  sed -nE 's/^[[:space:]]*CONFORMANCE_VERSION[[:space:]]*[?:]?=[[:space:]]*([^[:space:]#]+).*$/\1/p' \
    "$root/Makefile" | head -n1
)"

if [ $is_pseudo -eq 0 ]; then
  # Update Makefile pin
  update_make_var_line "$root/Makefile" "CONFORMANCE_VERSION" "$resolved_version"

  # Update nightly-tests “latest” entries (the ones matching current_conf)
  if [ -n "${current_conf:-}" ]; then
    update_nightly_gateway_api_matrix_versions \
      "$root/.github/workflows/nightly-tests.yaml" \
      "$current_conf" \
      "$resolved_version"
  else
    echo "WARN: Could not parse current CONFORMANCE_VERSION from Makefile; skipping workflow update"
  fi
else
  echo "WARN: Gateway API resolved to pseudo-version (${resolved_version}); leaving CONFORMANCE_VERSION and nightly matrix unchanged"
fi

if [ $is_pseudo -eq 0 ]; then
  # Try to download prebuilt install manifest from the GitHub release
  if [ "${CONFORMANCE_CHANNEL}" = "experimental" ]; then
    asset="experimental-install.yaml"
  else
    asset="standard-install.yaml"
  fi
  release_url="https://github.com/kubernetes-sigs/gateway-api/releases/download/${resolved_version}/${asset}"
  echo "Attempting to fetch Gateway API release asset: ${release_url}"
  tmp_release="$(mktemp)"
  if curl -fLSs "${release_url}" -o "${tmp_release}"; then
    # Extra safety: ensure non-empty before replacing
    if [ -s "${tmp_release}" ]; then
      mv -f "${tmp_release}" "${out_all}"
      echo "Wrote Gateway (${CONFORMANCE_CHANNEL}) CRDs to ${out_all} from release asset"
    else
      echo "Release asset downloaded but empty; falling back to repo iteration for ${ref}"
      rm -f "${tmp_release}"
      is_pseudo=1
    fi
  else
    echo "Release asset not available (or download failed); falling back to repo iteration for ${ref}"
    rm -f "${tmp_release}"
    is_pseudo=1
  fi
fi

if [ $is_pseudo -eq 1 ]; then
  # Fallback for pseudo-versions (or when release asset unavailable):
  # fetch all YAMLs under config/crd/${CONFORMANCE_CHANNEL} and concatenate.
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  archive_url="https://codeload.github.com/kubernetes-sigs/gateway-api/tar.gz/${ref}"
  echo "Downloading Gateway API archive: $archive_url"
  curl -fsSL "$archive_url" -o "$tmpdir/gtw.tar.gz"

  echo "Extracting archive..."
  tar -xzf "$tmpdir/gtw.tar.gz" -C "$tmpdir"

  srcdir="$(echo "$tmpdir"/gateway-api-*/config/crd/${CONFORMANCE_CHANNEL})"
  if [ ! -d "$srcdir" ]; then
    echo "ERROR: Could not find extracted CRD directory: $srcdir"
    exit 1
  fi

  tmp_all="$(mktemp)"
  : > "$tmp_all"
  echo "Building all-in-one CRD file at ${out_all} from ${srcdir}"
  # Concatenate in a stable order and include both x-k8s.io and k8s.io groups.
  for f in $(ls "$srcdir"/*.yaml | sort); do
    base="$(basename "$f")"
    echo "# Source: ${GATEWAY_CRD_BASE}/${ref}/config/crd/${CONFORMANCE_CHANNEL}/${base}" >> "$tmp_all"
    cat "$f" >> "$tmp_all"
    echo "---" >> "$tmp_all"
  done

  # Trim trailing separators/blank lines
  awk '{
    lines[NR]=$0
  } END {
    i=NR
    while (i>0 && (lines[i] ~ /^---[[:space:]]*$/ || lines[i] ~ /^[[:space:]]*$/)) { i-- }
    for (j=1; j<=i; j++) print lines[j]
  }' "$tmp_all" > "${out_all}"
  rm -f "$tmp_all"
  echo "Wrote Gateway (${CONFORMANCE_CHANNEL}) all-in-one CRDs to $out_all"
fi

# Final sanity: if ${out_all} exists but is empty, fail loudly
if [ -e "${out_all}" ] && [ ! -s "${out_all}" ]; then
  echo "ERROR: ${out_all} is empty after CRD refresh. Please check the logs above."
  exit 1
fi

echo "$kind bumped to ${resolved_version} (ref: ${ref}) successfully!"

#!/usr/bin/env bash
set -euo pipefail

# The GIE EPP used in e2e tests
EPP_YAML_PATH="test/kubernetes/e2e/features/inferenceextension/testdata/epp.yaml"

# The base URLs for fetching CRDs
GIE_CRD_BASE="https://raw.githubusercontent.com/kubernetes-sigs/gateway-api-inference-extension"
GATEWAY_CRD_BASE="https://raw.githubusercontent.com/kubernetes-sigs/gateway-api"

# The Gateway API channel (experimental or stable)
CONFORMANCE_CHANNEL="${CONFORMANCE_CHANNEL:-experimental}"

if [ $# -ne 2 ]; then
  echo "Usage: $0 {gie|gtw} REF"
  echo "  REF can be a tag (e.g. v1.3.0) or a commit SHA."
  exit 2
fi

kind="$1"; shift
ref="$1"; shift

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Bumping $kind to ref: $ref ..."

# Check the current module version
case "$kind" in
  gie) module="sigs.k8s.io/gateway-api-inference-extension";;
  gtw) module="sigs.k8s.io/gateway-api";;
  *) echo "Unknown kind: $kind" >&2; exit 1;;
esac

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

# Helper: derive image tag from module version.
# - For pseudo-versions that end with '...-YYYYMMDDHHMMSS-<sha>', produce 'vYYYYMMDD-<sha7>'
# - Otherwise (e.g., semver tag), use the version itself as the image tag.
derive_image_tag() {
  local ver="$1"
  # Match the trailing timestamp + sha anywhere at the end (robust for different pseudo-version bases)
  if [[ "$ver" =~ ([0-9]{14})-([0-9a-f]{7,40})$ ]]; then
    local ts="${BASH_REMATCH[1]}"
    local sha="${BASH_REMATCH[2]}"
    # Use YYYYMMDD and first 7 chars of sha
    echo "v${ts:0:8}-${sha:0:7}"
  else
    # Fall back to the version string itself (for true tags like v1.0.0 or v1.0.0-rc.2)
    echo "$ver"
  fi
}

# Update e2e EPP image tag (GIE only)
if [ "$kind" = gie ]; then
  img_tag="$(derive_image_tag "$resolved_version")"
  echo "Updating EPP image tag in $EPP_YAML_PATH to ${img_tag} (from module version: ${resolved_version})"
  # macOS/BSD-safe inline edit
  sed -i.bak -E \
    -e "s|(gateway-api-inference-extension/epp:)[^[:space:]\"]+|\1${img_tag}|g" \
    "$root/$EPP_YAML_PATH"
    rm -f "$root/$EPP_YAML_PATH.bak"
fi

# Fetch and store the CRDs
crd_dir="$root/internal/kgateway/crds"
mkdir -p "$crd_dir"

if [ "$kind" = gie ]; then
  # Build a single all-in-one CRD file.
  outfile="${crd_dir}/inference-crds.yaml"
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  # Known CRDs/sources
  declare -a SOURCES=(
    "${GIE_CRD_BASE}/${ref}/config/crd/bases/inference.networking.k8s.io_inferencepools.yaml"
    "${GIE_CRD_BASE}/${ref}/config/crd/bases/inference.networking.x-k8s.io_inferenceobjectives.yaml"
  )

  tmpout="$(mktemp)"
  : > "$tmpout"

  for url in "${SOURCES[@]}"; do
    fname="$tmpdir/$(basename "$url")"
    echo "Fetching $url"
    curl -fsS "$url" -o "$fname"
    {
      echo "# Source: $url"
      cat "$fname"
      echo "---"
    } >> "$tmpout"
  done

  # Remove the trailing '---' and any trailing blank lines
  # shellcheck disable=SC2016
  awk '{
    lines[NR]=$0
  } END {
    # drop trailing separators/blank lines
    i=NR
    while (i>0 && (lines[i] ~ /^---[[:space:]]*$/ || lines[i] ~ /^[[:space:]]*$/)) { i-- }
    for (j=1; j<=i; j++) print lines[j]
  }' "$tmpout" > "${tmpout}.trim"

  mv -f "${tmpout}.trim" "$outfile"
  rm -f "$tmpout"
  echo "Wrote GIE all-in-one CRDs to $outfile"

elif [ "$kind" = gtw ]; then
  # Gateway API CRDs
  # Use release assets when ref resolves to a non-pseudo version (tags incl. -rc.*),
  # otherwise iterate files from the repo for pseudo-versions (timestamps/SHAs).

  out_all="${crd_dir}/gateway-crds.yaml"

  is_pseudo=0
  if [[ "$resolved_version" =~ ([0-9]{14})-([0-9a-f]{7,40})$ ]]; then
    is_pseudo=1
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
        echo "Release asset downloaded but empty; falling back to repo iteration for ${resolved_version}"
        rm -f "${tmp_release}"
        is_pseudo=1
      fi
    else
      echo "Release asset not available (or download failed); falling back to repo iteration for ${resolved_version}"
      rm -f "${tmp_release}"      is_pseudo=1
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
fi

echo "$kind bumped to ${resolved_version} (ref: ${ref}) successfully!"

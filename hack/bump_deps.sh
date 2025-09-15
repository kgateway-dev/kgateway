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
  echo "Usage: $0 {gie|gtw} DEP_VERSION"
  exit 2
fi

kind="$1"; shift
ver="$1"; shift

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Bumping $kind to $ver..."

# Check the current module version
case "$kind" in
  gie) module="sigs.k8s.io/gateway-api-inference-extension";;
  gtw) module="sigs.k8s.io/gateway-api";;
  *) echo "Unknown kind: $kind" >&2; exit 1;;
esac

current="$(go list -m "$module" | awk '{print $2}')"
if [ "$current" = "$ver" ]; then
  echo "Current $kind version in go.mod is already $ver - skipping bump."
  exit 0
fi

# Bump the module deps
echo "Running go get $module@$ver"
go get "${module}@${ver}"
echo "Running go mod tidy"
go mod tidy

# Update e2e EPP image tag (GIE only)
if [ "$kind" = gie ]; then
  echo "Updating EPP image tag in $EPP_YAML_PATH"
  sed -i.bak -E \
    -e "s|(gateway-api-inference-extension/epp:)[^[:space:]\"]+|\1${ver}|g" \
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
    "${GIE_CRD_BASE}/${ver}/config/crd/bases/inference.networking.k8s.io_inferencepools.yaml"
    "${GIE_CRD_BASE}/${ver}/config/crd/bases/inference.networking.x-k8s.io_inferenceobjectives.yaml"
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
  # The all-in-one Gateway API CRDs
  url1="${GATEWAY_CRD_BASE}/${ver}/config/crd/${CONFORMANCE_CHANNEL}/gateway.networking.k8s.io_${CONFORMANCE_CHANNEL}.yaml"
  out1="${crd_dir}/gateway-crds.yaml"
  echo "Fetching $url1 and saving to $out1"
  curl -fsS "$url1" -o "$out1"

  # The separate TCPRoute CRD
  url2="${GATEWAY_CRD_BASE}/${ver}/config/crd/${CONFORMANCE_CHANNEL}/gateway.networking.k8s.io_tcproutes.yaml"
  out2="${crd_dir}/tcproute-crd.yaml"
  echo "Fetching $url2 and saving to $out2"
  curl -fsS "$url2" -o "$out2"
fi

echo "$kind bumped to $ver successfully!"

#!/usr/bin/env bash
# Installs kgateway for conformance testing with stats disabled via GatewayParameters.
# Mirrors the "Install kgateway via helm" step in .github/actions/kube-conformance-tests/action.yaml.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

VERSION="${VERSION:-}"
VERSION_EXPLICIT=false
API_CHANNEL="experimental"
ADDITIONAL_HELM_VALUES=()
IMAGE_REGISTRY="${IMAGE_REGISTRY:-ghcr.io/kgateway-dev}"

GWP_NAME="kgateway-no-stats"
GWP_NAMESPACE="kgateway-system"

usage() {
  cat <<EOF
Usage: $0 [OPTIONS]

Options:
  --version VERSION               Version of the kgateway and kgateway-crds charts.
                                  If omitted, uses local charts with VERSION from 'make print-VERSION'.
  --api-channel CHANNEL           Gateway API channel: 'experimental' (default) or 'standard'.
                                  'standard' disables experimental Gateway API features.
  --additional-helm-values PATH   Path to an additional Helm values file. May be repeated.
  --image-registry REGISTRY       Image registry (default: ghcr.io/kgateway-dev).
  -h, --help                      Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"; VERSION_EXPLICIT=true; shift 2 ;;
    --api-channel)
      API_CHANNEL="$2"; shift 2 ;;
    --additional-helm-values)
      ADDITIONAL_HELM_VALUES+=("$2"); shift 2 ;;
    --image-registry)
      IMAGE_REGISTRY="$2"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "Unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$VERSION" ]]; then
  VERSION="$(make -C "$REPO_ROOT" print-VERSION)"
fi

EXPERIMENTAL_FLAG=""
if [[ "$API_CHANNEL" == "standard" ]]; then
  EXPERIMENTAL_FLAG="--set controller.extraEnv.KGW_ENABLE_EXPERIMENTAL_GATEWAY_API_FEATURES=false"
fi

ADDITIONAL_VALUES_FLAGS=()
for values_file in ${ADDITIONAL_HELM_VALUES[@]+"${ADDITIONAL_HELM_VALUES[@]}"}; do
  if [[ ! -f "$values_file" ]]; then
    echo "Error: additional-helm-values file not found: $values_file" >&2
    exit 1
  fi
  ADDITIONAL_VALUES_FLAGS+=(-f "$values_file")
done

echo "==> Installing kgateway-crds (version=${VERSION})"
if [[ "$VERSION_EXPLICIT" == "false" ]]; then
  helm upgrade -i -n "$GWP_NAMESPACE" kgateway-crds "$REPO_ROOT/install/helm/kgateway-crds/" \
    --create-namespace
else
  helm upgrade -i -n "$GWP_NAMESPACE" kgateway-crds "oci://${IMAGE_REGISTRY}/charts/kgateway-crds" \
    --version "${VERSION}" \
    --create-namespace
fi

echo "==> Creating GatewayParameters '${GWP_NAME}' in '${GWP_NAMESPACE}' (stats.enabled=false)"
kubectl apply -f - <<EOF
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: ${GWP_NAME}
  namespace: ${GWP_NAMESPACE}
spec:
  kube:
    stats:
      enabled: false
EOF

echo "==> Installing kgateway (version=${VERSION})"
if [[ "$VERSION_EXPLICIT" == "false" ]]; then
  helm upgrade -i -n "$GWP_NAMESPACE" kgateway "$REPO_ROOT/install/helm/kgateway/" \
    --create-namespace \
    ${EXPERIMENTAL_FLAG} \
    --set image.tag="${VERSION}" \
    --set image.registry="${IMAGE_REGISTRY}" \
    --set "gatewayClassParametersRefs.kgateway.name=${GWP_NAME}" \
    --set "gatewayClassParametersRefs.kgateway.namespace=${GWP_NAMESPACE}" \
    ${ADDITIONAL_VALUES_FLAGS[@]+"${ADDITIONAL_VALUES_FLAGS[@]}"}
else
  helm upgrade -i -n "$GWP_NAMESPACE" kgateway "oci://${IMAGE_REGISTRY}/charts/kgateway" \
    --version "${VERSION}" \
    --create-namespace \
    ${EXPERIMENTAL_FLAG} \
    --set image.tag="${VERSION}" \
    --set "gatewayClassParametersRefs.kgateway.name=${GWP_NAME}" \
    --set "gatewayClassParametersRefs.kgateway.namespace=${GWP_NAMESPACE}" \
    ${ADDITIONAL_VALUES_FLAGS[@]+"${ADDITIONAL_VALUES_FLAGS[@]}"}
fi

echo "==> Done. kgateway installed with GatewayParameters '${GWP_NAME}' (stats disabled)."

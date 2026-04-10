#!/usr/bin/env bash
#
# Setup script for kgateway on a kind cluster using released artifacts.
#
# This is useful for quickly reproducing bugs or testing released versions
# the same way a user would, without building from source.
#
# Usage: ./hack/setup-kind-via-release.sh [options]
#
# This script:
#   1. Creates a kind cluster
#   2. Optionally installs MetalLB for LoadBalancer support
#   3. Installs Gateway API CRDs (standard or experimental)
#   4. Installs kgateway via helm from released OCI charts
#   5. Optionally creates a GatewayClass, Gateway, and DirectResponse smoke test
#

set -euo pipefail

# --- Defaults ---

cluster_name=""
kgw_version="v2.3.0-main"
helm_registry="oci://cr.kgateway.dev/kgateway-dev/charts"
namespace="kgateway-system"
k8s_version=""
gateway_api_version="v1.2.1"
gateway_api_channel="standard"
enable_metallb=false
metallb_version="v0.13.7"
enable_cloud_provider_kind=false
enable_gateway=true
gateway_name="kgw"
gateway_class_name="kgateway"
kind_cmd="go tool kind"
helm_cmd="go tool helm"

# --- Usage ---

usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

Sets up kgateway on a kind cluster using released artifacts.

Options:
  -h, --help                     Show this help message
  -c, --cluster-name NAME        Kind cluster name             (default: kind-kgw-<version>-gw-<gw-version>-<channel>)
  -v, --version VERSION          kgateway helm chart version    (default: v2.3.0-main)
  -r, --registry URL             Helm OCI registry              (default: oci://cr.kgateway.dev/kgateway-dev/charts)
  -n, --namespace NS             Install namespace              (default: kgateway-system)
  -k, --k8s-version VER          kindest/node image tag (see https://hub.docker.com/r/kindest/node/tags)
  --gateway-api-version VER      Gateway API CRD version        (default: v1.2.1)
  --gateway-api-channel CHAN     standard or experimental       (default: standard)
  --metallb                      Install MetalLB (off by default)
  --metallb-version VER          MetalLB version                (default: v0.13.7)
  --cloud-provider-kind          Run cloud-provider-kind for LoadBalancer support (off by default)
  --no-gateway                   Skip GatewayClass/Gateway/HTTPRoute creation
  --gateway-name NAME            Name for the Gateway           (default: kgw)
  --gateway-class-name NAME      Name for the GatewayClass      (default: kgateway)

Examples:
  # Default: latest rolling main build
  ./hack/setup-kind-via-release.sh

  # Specific release with experimental channel
  ./hack/setup-kind-via-release.sh -v v2.1.0 --gateway-api-channel experimental

  # Specific k8s version with MetalLB
  ./hack/setup-kind-via-release.sh -k v1.31.12 --metallb

  # With cloud-provider-kind for LoadBalancer IP assignment
  ./hack/setup-kind-via-release.sh --cloud-provider-kind

  # Install kgateway only, no Gateway resources
  ./hack/setup-kind-via-release.sh --no-gateway
EOF
    exit 0
}

# --- Argument parsing ---

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            usage ;;
        -c|--cluster-name)
            cluster_name="$2"; shift 2 ;;
        -v|--version)
            kgw_version="$2"; shift 2 ;;
        -r|--registry)
            helm_registry="$2"; shift 2 ;;
        -n|--namespace)
            namespace="$2"; shift 2 ;;
        -k|--k8s-version)
            k8s_version="$2"; shift 2 ;;
        --gateway-api-version)
            gateway_api_version="$2"; shift 2 ;;
        --gateway-api-channel)
            gateway_api_channel="$2"; shift 2 ;;
        --metallb)
            enable_metallb=true; shift ;;
        --metallb-version)
            metallb_version="$2"; shift 2 ;;
        --cloud-provider-kind)
            enable_cloud_provider_kind=true; shift ;;
        --no-gateway)
            enable_gateway=false; shift ;;
        --gateway-name)
            gateway_name="$2"; shift 2 ;;
        --gateway-class-name)
            gateway_class_name="$2"; shift 2 ;;
        *)
            echo "Unknown option: $1" >&2
            echo "Run $(basename "$0") --help for usage." >&2
            exit 1 ;;
    esac
done

# Normalize version inputs: strip leading "v" so we can add it consistently.
kgw_version="${kgw_version#v}"
gateway_api_version="${gateway_api_version#v}"

# Compute default cluster name from versions if not explicitly set.
# e.g. kind-kgw-2.3.0-main-gw-1.2.1-standard
if [[ -z "${cluster_name}" ]]; then
    cluster_name="kind-kgw-${kgw_version}-gw-${gateway_api_version}-${gateway_api_channel}"
fi

# --- Functions ---

create_kind_cluster() {
    if $kind_cmd get clusters 2>/dev/null | grep -q "^${cluster_name}$"; then
        echo "Kind cluster '${cluster_name}' already exists, using existing cluster"
    else
        echo "Creating kind cluster '${cluster_name}'..."
        local kind_args=(create cluster --name "${cluster_name}" --wait 60s)
        if [[ -n "${k8s_version}" ]]; then
            kind_args+=(--image "kindest/node:${k8s_version}")
        fi
        $kind_cmd "${kind_args[@]}"
    fi

    kubectl config use-context "kind-${cluster_name}"
    echo "Waiting for cluster nodes to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s
}

maybe_install_metallb() {
    if [[ "${enable_metallb}" != "true" ]]; then
        echo "Skipping MetalLB (use --metallb to install)"
        return
    fi

    echo "Installing MetalLB ${metallb_version}..."
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    METALLB_VERSION="${metallb_version}" . "${script_dir}/kind/setup-metalllb-on-kind.sh"
    echo "MetalLB configured"
}

maybe_start_cloud_provider_kind() {
    if [[ "${enable_cloud_provider_kind}" != "true" ]]; then
        echo "Skipping cloud-provider-kind (use --cloud-provider-kind to enable)"
        return
    fi

    echo "Starting cloud-provider-kind..."
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    . "${script_dir}/kind/setup-cloud-provider-kind.sh"
}

install_gateway_api_crds() {
    echo "Installing Gateway API CRDs v${gateway_api_version} (${gateway_api_channel} channel)..."
    kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/v${gateway_api_version}/${gateway_api_channel}-install.yaml"
}

install_kgateway() {
    echo "Installing kgateway-crds v${kgw_version}..."
    $helm_cmd upgrade -i --create-namespace \
        --namespace "${namespace}" \
        --version "v${kgw_version}" \
        kgateway-crds \
        "${helm_registry}/kgateway-crds" \
        --wait

    echo "Installing kgateway v${kgw_version}..."
    $helm_cmd upgrade -i --create-namespace \
        --namespace "${namespace}" \
        --version "v${kgw_version}" \
        kgateway \
        "${helm_registry}/kgateway" \
        --wait

    echo "Waiting for kgateway controller to be ready..."
    kubectl rollout status deployment/kgateway -n "${namespace}" --timeout=120s
}

maybe_create_gateway() {
    if [[ "${enable_gateway}" != "true" ]]; then
        echo "Skipping Gateway/GatewayClass creation"
        return
    fi

    echo "Creating GatewayClass '${gateway_class_name}'..."
    kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: ${gateway_class_name}
spec:
  controllerName: kgateway.dev/kgateway
EOF

    echo "Creating Gateway '${gateway_name}'..."
    kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ${gateway_name}
  namespace: default
spec:
  gatewayClassName: ${gateway_class_name}
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: All
EOF

    echo "Creating DirectResponse and HTTPRoute for smoke testing..."
    kubectl apply -f - <<EOF
apiVersion: gateway.kgateway.dev/v1alpha1
kind: DirectResponse
metadata:
  name: hello
  namespace: default
spec:
  status: 200
  body: "kgateway is running"
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: hello
  namespace: default
spec:
  parentRefs:
    - name: ${gateway_name}
  rules:
    - matches:
        - path:
            type: Exact
            value: /healthz
      filters:
        - type: ExtensionRef
          extensionRef:
            group: gateway.kgateway.dev
            kind: DirectResponse
            name: hello
EOF
}

# --- Main ---

echo "=== Setting up kgateway v${kgw_version} on kind cluster '${cluster_name}' ==="
echo "  Gateway API: v${gateway_api_version} (${gateway_api_channel})"
echo "  Namespace:   ${namespace}"
echo ""

create_kind_cluster
maybe_install_metallb
maybe_start_cloud_provider_kind
install_gateway_api_crds
install_kgateway
maybe_create_gateway

echo ""
echo "=== Setup Complete ==="
echo ""

if [[ "${enable_gateway}" == "true" ]]; then
    echo "Waiting for Gateway deployment..."
    sleep 5

    if kubectl get deployment "${gateway_name}" -n default &>/dev/null; then
        echo "Gateway deployment created:"
        kubectl get deployment "${gateway_name}" -n default
    else
        echo "Gateway deployment not yet created (may take a few more seconds)."
        echo "Check with: kubectl get deployment -n default"
    fi

    echo ""
    echo "Smoke test (port-forward):"
    echo "  kubectl port-forward -n default svc/${gateway_name} 8080:8080 &"
    echo "  curl http://localhost:8080/healthz"
fi

echo ""
echo "Useful commands:"
echo "  kubectl get gateway -A"
echo "  kubectl get gatewayclass"
echo "  kubectl get pods -n ${namespace}"
echo "  kubectl get deployment -n default"
echo ""
echo "To tear down:"
echo "  ${kind_cmd} delete cluster --name ${cluster_name}"

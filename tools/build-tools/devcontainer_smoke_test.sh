#!/usr/bin/env bash
# Devcontainer smoke test: verifies that `make run` works inside the
# build-tools / devcontainer image and that the kgateway stack comes up.
#
# This script is designed to run inside the devcontainer (or an equivalent
# container image) with the Docker socket mounted from the host.

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kind}"

# ---------- helpers ----------

info()  { echo "==> $*"; }
fail()  { echo "FAIL: $*" >&2; exit 1; }

wait_for_pods() {
  local namespace="$1" timeout_secs="$2"
  info "Waiting up to ${timeout_secs}s for all pods in ${namespace} to be Ready..."
  if ! kubectl wait pods --all -n "${namespace}" --for=condition=Ready --timeout="${timeout_secs}s"; then
    echo "--- pod status in ${namespace} ---"
    kubectl get pods -n "${namespace}" -o wide
    fail "Pods in ${namespace} not ready within ${timeout_secs}s"
  fi
}

cleanup() {
  info "Cleaning up kind cluster '${CLUSTER_NAME}'..."
  kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
}

# fix_kubeconfig rewrites the kubeconfig so kubectl can reach the kind API
# server from inside a Docker container. When --network=host works (Linux),
# this is a no-op because 127.0.0.1 already resolves correctly. On macOS
# Docker Desktop, --network=host is a no-op so we must connect to the kind
# Docker network and use the control-plane container's IP instead.
fix_kubeconfig() {
  # Quick check: can we already reach the API server?
  if kubectl cluster-info &>/dev/null; then
    return 0
  fi

  info "API server unreachable at 127.0.0.1; fixing kubeconfig for Docker networking..."

  local cp_container="${CLUSTER_NAME}-control-plane"
  local cp_ip
  cp_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${cp_container}" 2>/dev/null) \
    || fail "Cannot determine control-plane container IP"

  # Connect this container to the kind Docker network so we can reach the
  # control-plane container directly.
  local self_id
  self_id=$(cat /proc/self/cgroup 2>/dev/null | grep -oP 'docker/\K[0-9a-f]+' | head -1 || true)
  if [[ -z "${self_id}" ]]; then
    # cgroup v2 fallback
    self_id=$(basename "$(cat /proc/1/cpuset 2>/dev/null)" || true)
  fi
  if [[ -n "${self_id}" ]]; then
    docker network connect kind "${self_id}" 2>/dev/null || true
  fi

  # Rewrite the kubeconfig server URL to point at the control-plane container IP.
  local kubeconfig="${HOME}/.kube/config"
  if [[ -f "${kubeconfig}" ]]; then
    sed -i "s|https://127\.0\.0\.1:[0-9]*|https://${cp_ip}:6443|g" "${kubeconfig}"
    info "Rewrote kubeconfig to use ${cp_ip}:6443"
  fi

  # Verify connectivity after fix
  kubectl cluster-info || fail "Still cannot reach API server after kubeconfig fix"
}

# ---------- auto-negotiate Docker API version ----------

if [[ -z "${DOCKER_API_VERSION:-}" ]] && command -v docker &>/dev/null; then
  _server_api=$(docker version --format '{{.Server.APIVersion}}' 2>/dev/null || true)
  if [[ -n "${_server_api}" ]]; then
    export DOCKER_API_VERSION="${_server_api}"
    info "Negotiated DOCKER_API_VERSION=${DOCKER_API_VERSION}"
  fi
  unset _server_api
fi

# ---------- pre-flight ----------

info "Verifying Docker connectivity..."
docker info > /dev/null || fail "Cannot connect to Docker daemon"

# Mark the workspace as a safe git directory so Go VCS stamping works when the
# repo is bind-mounted from a different uid (e.g. CI runner -> container).
git config --global --add safe.directory /workspace 2>/dev/null || true

# ---------- create kind cluster and fix networking ----------

trap cleanup EXIT

info "Creating kind cluster..."
make kind-create CLUSTER_NAME="${CLUSTER_NAME}"

fix_kubeconfig

# ---------- make run (continues from existing cluster) ----------

info "Running 'make run' (this will take a while)..."
make run CLUSTER_NAME="${CLUSTER_NAME}"

# ---------- wait for system pods ----------

wait_for_pods kube-system 120
wait_for_pods metallb-system 120

# ---------- wait for kgateway controller ----------

wait_for_pods kgateway-system 180

# ---------- deploy a Gateway and verify proxy pod ----------

info "Applying a test Gateway resource..."
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: smoke-test
  namespace: default
spec:
  gatewayClassName: kgateway
  listeners:
    - name: http
      protocol: HTTP
      port: 8080
EOF

info "Waiting for envoy proxy pod to become Ready..."
for i in $(seq 1 60); do
  if kubectl get pods -n default -l "gateway.networking.k8s.io/gateway-name=smoke-test" --no-headers 2>/dev/null | grep -q .; then
    break
  fi
  sleep 5
done

kubectl wait pods -n default -l "gateway.networking.k8s.io/gateway-name=smoke-test" \
  --for=condition=Ready --timeout=180s \
  || fail "Envoy proxy pod for smoke-test Gateway not ready"

# ---------- done ----------

info "Devcontainer smoke test PASSED"

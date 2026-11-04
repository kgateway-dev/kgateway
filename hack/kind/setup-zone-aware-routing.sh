#!/usr/bin/env bash

set -ex

# Get directory this script is located in to access script local files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# The name of the kind cluster to deploy to
CLUSTER_NAME="${CLUSTER_NAME:-kgw-zone-aware}"
# The version of the Node Docker image to use for booting the cluster: https://hub.docker.com/r/kindest/node/tags
# This version should stay in sync with `setup-kind.sh` and `Makefile`.
CLUSTER_NODE_VERSION="${CLUSTER_NODE_VERSION:-v1.35.0@sha256:452d707d4862f52530247495d180205e029056831160e22870e37e3f6c1ac31f}"
# The kind CLI to use. Defaults to the latest version from the kind repo.
KIND="${KIND:-go tool kind}"

# The test stub region and zones assigned to the worker nodes.
ZONE_REGION="us-east-1"
ZONES=(us-east-1a us-east-1b us-east-1c)

# Mirrors create_kind_cluster_or_skip in setup-kind.sh, with a multi-node config for zone-aware routing.
function create_kind_cluster_or_skip() {
  activeClusters=$($KIND get clusters)

  # if the kind cluster exists already, return
  if [[ "$activeClusters" =~ .*"$CLUSTER_NAME".* ]]; then
    echo "cluster exists, skipping cluster creation"
    return
  fi

  echo "creating cluster ${CLUSTER_NAME}"
  $KIND create cluster \
    --name "$CLUSTER_NAME" \
    --image "kindest/node:$CLUSTER_NODE_VERSION" \
    --config=- <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
- role: worker
EOF
  echo "Finished setting up cluster $CLUSTER_NAME"
}

function label_worker_nodes() {
  # Label each worker node with its own zone so that gateway and backend pods
  # scheduled to it get the corresponding locality.
  workers=("${CLUSTER_NAME}-worker" "${CLUSTER_NAME}-worker2" "${CLUSTER_NAME}-worker3")

  for i in "${!workers[@]}"; do
    kubectl label node "${workers[$i]}" \
      "topology.kubernetes.io/region=${ZONE_REGION}" \
      "topology.kubernetes.io/zone=${ZONES[$i]}" \
      --overwrite
  done
}

function main() {
  cd "${ROOT_DIR}"

  create_kind_cluster_or_skip

  kubectl config use-context "kind-${CLUSTER_NAME}"
  kubectl wait --for=condition=Ready nodes --all --timeout=180s

  label_worker_nodes

  # Reuse the standard workflow.
  CLUSTER_NAME="${CLUSTER_NAME}" make run

  kubectl rollout status deployment/kgateway -n kgateway-system --timeout=180s

  set +x
  cat <<EOF

Zone-aware routing test cluster is ready.

Run the e2e test with:

  ZONE_AWARE_CLUSTER_NAME=${CLUSTER_NAME} go test -tags=e2e -vet=off -timeout=20m ./test/e2e/tests -run '^TestZoneAwareRouting$' -count=1

EOF
}

main

#!/usr/bin/env bash

set -ex

# 0. Assign default values to some of our environment variables
# Get directory this script is located in to access script local files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
# The name of the k3d cluster to deploy to
CLUSTER_NAME="${CLUSTER_NAME:-k3d}"
# The Kubernetes node version (e.g. v1.35.0 or v1.35.0@sha256:...).
# Strip any @sha256 suffix to get the semver, then build the k3s image tag.
NODE_VERSION="${NODE_VERSION:-v1.35.0}"
K3S_SEMVER="${NODE_VERSION%%@*}"
K3D_NODE_IMAGE="${K3D_NODE_IMAGE:-rancher/k3s:${K3S_SEMVER}-k3s1}"
# The version used to tag images
VERSION="${VERSION:-v1.0.0-ci1}"
# Skip building docker images if we are testing a released version
SKIP_DOCKER="${SKIP_DOCKER:-false}"
# Load prebuilt docker images into the cluster instead of rebuilding them
LOAD_DOCKER_IMAGES="${LOAD_DOCKER_IMAGES:-false}"
# Stop after creating the k3d cluster
JUST_K3D="${JUST_K3D:-false}"
# The version of the k8s gateway api conformance tests to run.
CONFORMANCE_VERSION="${CONFORMANCE_VERSION:-$(go list -m sigs.k8s.io/gateway-api | awk '{print $2}')}"
# The channel of the k8s gateway api conformance tests to run.
CONFORMANCE_CHANNEL="${CONFORMANCE_CHANNEL:-"experimental"}"
# The k3d CLI to use.
K3D="${K3D:-k3d}"
# The helm CLI to use. Defaults to the latest version from the helm repo.
HELM="${HELM:-go tool helm}"
# If true, use localstack for lambda functions
LOCALSTACK="${LOCALSTACK:-false}"
# Registry cache reference for envoyinit Docker build (optional)
ENVOYINIT_CACHE_REF="${ENVOYINIT_CACHE_REF:-}"

# Export the variables so they are available in the environment
export VERSION CLUSTER_NAME CLUSTER_TYPE=k3d ENVOYINIT_CACHE_REF

function create_k3d_cluster_or_skip() {
  # If the k3d cluster exists already, return
  if $K3D cluster list -o json | jq -e ".[] | select(.name==\"$CLUSTER_NAME\")" > /dev/null 2>&1; then
    echo "cluster exists, skipping cluster creation"
    return
  fi

  echo "creating cluster ${CLUSTER_NAME}"
  $K3D cluster create "$CLUSTER_NAME" \
    --image "$K3D_NODE_IMAGE" \
    --k3s-arg "--disable=traefik@server:0" \
    --k3s-arg "--disable=servicelb@server:0" \
    -p "80:80@loadbalancer" \
    -p "443:443@loadbalancer"
  echo "Finished setting up cluster $CLUSTER_NAME"

  # so that you can just build the k3d cluster alone if needed
  if [[ $JUST_K3D == 'true' ]]; then
    echo "JUST_K3D=true, not building images"
    exit
  fi
}

function create_and_setup() {
  create_k3d_cluster_or_skip

  # Apply the Kubernetes Gateway API CRDs
  # Use release URL for version tags (faster, avoiding 27s timeout), but use
  # kustomize for commit SHAs -- this is needed to run conformance tests from
  # main when either dependency references a pseudo-version instead of a
  # release.
  if [[ $CONFORMANCE_VERSION =~ ^v[0-9] ]]; then
    kubectl apply --server-side -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/$CONFORMANCE_VERSION/$CONFORMANCE_CHANNEL-install.yaml"
  elif [[ $CONFORMANCE_CHANNEL == "standard" ]]; then
    kubectl apply --server-side --kustomize "https://github.com/kubernetes-sigs/gateway-api/config/crd?ref=$CONFORMANCE_VERSION"
  else
    kubectl apply --server-side --kustomize "https://github.com/kubernetes-sigs/gateway-api/config/crd/$CONFORMANCE_CHANNEL?ref=$CONFORMANCE_VERSION"
  fi

  # Start the lightweight LoadBalancer IP assigner in the background.
  # k3s ServiceLB uses host ports, causing conflicts when multiple services share
  # the same port. This script assigns unique IPs from the Docker network instead.
  # Only launch if one is not already running for this cluster.
  if pgrep -f "k3d-loadbalancer.sh ${CLUSTER_NAME}$" > /dev/null 2>&1; then
    echo "k3d load balancer assigner already running for cluster ${CLUSTER_NAME}"
  else
    nohup "$SCRIPT_DIR/k3d-loadbalancer.sh" "$CLUSTER_NAME" > /tmp/k3d-lb-"${CLUSTER_NAME}".log 2>&1 &
    disown
  fi
}

# 1. Create a k3d cluster (or skip creation if a cluster with name=CLUSTER_NAME already exists)
create_and_setup

if [[ $SKIP_DOCKER == 'true' ]]; then
  echo "SKIP_DOCKER=true, not building images"
  if [[ $LOAD_DOCKER_IMAGES == 'true' ]]; then
    VERSION=$VERSION K3D_CLUSTER_NAME=$CLUSTER_NAME make k3d-load
  fi
else
  # 2. Make all the docker images and load them to the k3d cluster
  VERSION=$VERSION K3D_CLUSTER_NAME=$CLUSTER_NAME make k3d-build-and-load k3d-load-dummy-idp
fi

# 3. Setup localstack
if [[ $LOCALSTACK == "true" ]]; then
  echo "Setting up localstack"
  . "$SCRIPT_DIR/../setup-localstack.sh"
fi

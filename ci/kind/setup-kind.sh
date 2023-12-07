#!/bin/bash -ex

# 0. Assign default values to some of our environment variables
# Get directory this script is located in to access script local files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
# The name of the kind cluster to deploy to
CLUSTER_NAME="${CLUSTER_NAME:-kind}"
# The version of the Node Docker image to use for booting the cluster
CLUSTER_NODE_VERSION="${CLUSTER_NODE_VERSION:-v1.28.0}"
# The version used to tag images
VERSION="${VERSION:-1.0.0-ci}"
# Skip building docker images if we are testing a released version
SKIP_DOCKER="${SKIP_DOCKER:-false}"
# Stop after creating the kind cluster
JUST_KIND="${JUST_KIND:-false}"
# Offer a default value for type of installation
KUBE2E_TESTS="${KUBE2E_TESTS:-gateway}"  # If 'KUBE2E_TESTS' not set or null, use 'gateway'.
# The version of istio to install for glooctl tests
# https://istio.io/latest/docs/releases/supported-releases/#support-status-of-istio-releases
ISTIO_VERSION="${ISTIO_VERSION:-1.18.2}"

function create_kind_cluster_or_skip() {
  activeClusters=$(kind get clusters)

  # if the kind cluster exists already, return
  if [[ "$activeClusters" =~ .*"$CLUSTER_NAME".* ]]; then
    echo "cluster exists, skipping cluster creation"
    return
  fi

  echo "creating cluster ${CLUSTER_NAME}"
  kind create cluster \
    --name "$CLUSTER_NAME" \
    --image "kindest/node:$CLUSTER_NODE_VERSION" \
    --config="$SCRIPT_DIR/cluster.yaml"
  echo "Finished setting up cluster $CLUSTER_NAME"

  # so that you can just build the kind image alone if needed
  if [[ $JUST_KIND == 'true' ]]; then
    echo "JUST_KIND=true, not building images"
    exit
  fi
}

# 1. Create a kind cluster (or skip creation if a cluster with name=CLUSTER_NAME already exists)
# This config is roughly based on: https://kind.sigs.k8s.io/docs/user/ingress/
create_kind_cluster_or_skip

if [[ $SKIP_DOCKER == 'true' ]]; then
  echo "SKIP_DOCKER=true, not building images or chart"
else
  # 2. Make all the docker images and load them to the kind cluster
  VERSION=$VERSION CLUSTER_NAME=$CLUSTER_NAME USE_SILENCE_REDIRECTS=true make -s kind-build-and-load

  # 3. Build the test helm chart, ensuring we have a chart in the `_test` folder
  VERSION=$VERSION USE_SILENCE_REDIRECTS=true make -s build-test-chart
fi

# 4. Build the gloo command line tool, ensuring we have one in the `_output` folder
USE_SILENCE_REDIRECTS=true make -s build-cli-local

# 5. Install additional resources used for particular KUBE2E tests
if [[ $KUBE2E_TESTS = "glooctl" || $KUBE2E_TESTS = "istio" ]]; then
  TARGET_ARCH=x86_64
  if [[ $ARCH == 'arm64' ]]; then
    TARGET_ARCH=arm64
  fi
  echo "Downloading Istio $ISTIO_VERSION"
  curl -L https://istio.io/downloadIstio | ISTIO_VERSION=$ISTIO_VERSION TARGET_ARCH=$TARGET_ARCH sh -

  echo "Installing Istio"
  yes | "./istio-$ISTIO_VERSION/bin/istioctl" install --set profile=minimal
fi

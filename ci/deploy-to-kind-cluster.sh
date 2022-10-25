#!/bin/bash -ex

# 0. Assign default values to some of our environment variables
# The name of the kind cluster to deploy to
CLUSTER_NAME="${CLUSTER_NAME:-kind}"
# The version of the Node Docker image to use for booting the cluster
CLUSTER_NODE_VERSION="${CLUSTER_NODE_VERSION:-v1.22.4}"
# The version used to tag images
VERSION="${VERSION:-0.0.0-kind1}"
# Automatically (lazily) determine OS type
if [[ $OSTYPE == 'darwin'* ]]; then
  OS='darwin'
else
  OS='linux'
fi
# set the architecture of the machine
UNAME_M=$(uname -m)
ARCH="amd64"
if [[ $UNAME_M == 'arm64' || $UNAME_M == 'aarch64' ]]; then
  ARCH='arm64'
fi


# Offer a default value for type of installation
KUBE2E_TESTS="${KUBE2E_TESTS:-gateway}"  # If 'KUBE2E_TESTS' not set or null, use 'gateway'.
# The version of istio to install for glooctl tests
# https://istio.io/latest/docs/releases/supported-releases/#support-status-of-istio-releases
ISTIO_VERSION="${ISTIO_VERSION:-1.11.4}"

# if user is running arm, these are configurations for the registry
REGISTRY_NAME='kind-registry'
REGISTRY_PORT="${REGISTRY_PORT:-5000}"

function create_kind_registry() {
  # create registry container unless it already exists
  if [ "$(docker inspect -f '{{.State.Running}}' "${REGISTRY_NAME}" 2>/dev/null || true)" != 'true' ]; then
    docker run \
      -d --restart=always -p "127.0.0.1:${REGISTRY_PORT}:5000" --name "${REGISTRY_NAME}" \
      registry:2
  fi
}

function connectKindNetworkToRegistry() {
  # connect the registry to the cluster network if not already connected
  if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${REGISTRY_NAME}")" = 'null' ]; then
    docker network connect "kind" "${REGISTRY_NAME}"
  fi
}

function createKubeRegistryConfigMap() {
  # Document the local registry
  # https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

# 1. Create a kind cluster (or skip creation if a cluster with name=CLUSTER_NAME already exists)
# This config is roughly based on: https://kind.sigs.k8s.io/docs/user/ingress/
function create_kind_cluster_or_skip() {
  activeClusters=$(kind get clusters)

  # if the kind cluster exists already, return
  if [[ "$activeClusters" =~ .*"$CLUSTER_NAME".* ]]; then
    echo "cluster exists"
    return
  fi

  # create kind registry with endpoint
  if [[ $ARCH == "arm64" ]]; then
    create_kind_registry
    ARM_EXTENSION="containerdConfigPatches:
- |-
  [plugins.\"io.containerd.grpc.v1.cri\".registry.mirrors.\"localhost:${REGISTRY_PORT}\"]
    endpoint = [\"http://${REGISTRY_NAME}:5000\"]
"
  fi

  echo "creating cluster ${CLUSTER_NAME}"

 cat <<EOF | kind create cluster --name "$CLUSTER_NAME" --image "kindest/node:$CLUSTER_NODE_VERSION" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
kubeadmConfigPatches:
- |
  apiVersion: kubeadm.k8s.io/v1beta2
  kind: ClusterConfiguration
  metadata:
    name: config
  apiServer:
    extraArgs:
      "feature-gates": "EphemeralContainers=true"
  scheduler:
    extraArgs:
      "feature-gates": "EphemeralContainers=true"
  controllerManager:
    extraArgs:
      "feature-gates": "EphemeralContainers=true"
- |
  apiVersion: kubeadm.k8s.io/v1beta2
  kind: InitConfiguration
  metadata:
    name: config
  nodeRegistration:
    kubeletExtraArgs:
      "feature-gates": "EphemeralContainers=true"
$ARM_EXTENSION
EOF

  # finish setting up registry for arm64
  if [[ $ARCH == "arm64" ]]; then
    connectKindNetworkToRegistry
    createKubeRegistryConfigMap
  fi
  echo "Finished setting up cluster $CLUSTER_NAME"
}
create_kind_cluster_or_skip

# so that you can just build the kind image alone if needed
if [[ $JUST_KIND == 'true' ]]; then
  exit
fi

# 2. Make all the docker images and load them to the kind cluster
if [[ $ARCH == 'arm64' ]]; then
  # if using arm64, push to the docker registry container, instead of kind
  VERSION=$VERSION CREATE_TEST_ASSETS="true" TEST_ASSET_ID="docker-reg" IMAGE_REPO="localhost:$REGISTRY_PORT" make docker-push-local-arm -B
else
  VERSION=$VERSION CLUSTER_NAME=$CLUSTER_NAME make push-kind-images
fi

# 3. Build the test helm chart, ensuring we have a chart in the `_test` folder
RUNNING_REGRESSION_TESTS=true VERSION=$VERSION IMAGE_REPO="localhost:$REGISTRY_PORT" make build-test-chart

# 4. Build the gloo command line tool, ensuring we have one in the `_output` folder
make glooctl-$OS-$ARCH

# 5. Install additional resources used for particular KUBE2E tests
if [ "$KUBE2E_TESTS" = "glooctl" ]; then
  TARGET_ARCH=x86_64
  if [[ $ARCH == 'arm64' ]]; then
    TARGET_ARCH=arm64
  fi
  echo "Downloading Istio $ISTIO_VERSION"
  curl -L https://istio.io/downloadIstio | ISTIO_VERSION=$ISTIO_VERSION TARGET_ARCH=$TARGET_ARCH sh -

  echo "Installing Istio"
  yes | "./istio-$ISTIO_VERSION/bin/istioctl" install --set profile=minimal
fi

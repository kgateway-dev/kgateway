#!/bin/bash -ex

# 0. Assign default values to some of our environment variables
# The name of the kind cluster to deploy to
CLUSTER_NAME="${CLUSTER_NAME:-kind}"
# The version of the Node Docker image to use for booting the cluster
CLUSTER_NODE_VERSION="${CLUSTER_NODE_VERSION:-v1.17.17@sha256:66f1d0d91a88b8a001811e2f1054af60eef3b669a9a74f9b6db871f2f1eeed00}"
# The version used to tag images
VERSION="${VERSION:-0.0.0-kind}"

# 1. Create a kind cluster (or skip creation if a cluster with name=CLUSTER_NAME already exists)
# This config is roughly based on: https://kind.sigs.k8s.io/docs/user/ingress/
function create_kind_cluster_or_skip() {
  echo "creating cluster ${CLUSTER_NAME}"

  activeClusters=$(kind get clusters)

  if [[ "$activeClusters" =~ .*"$CLUSTER_NAME".* ]]; then
    echo "cluster exists"
    return
  fi

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
EOF

  echo "Finished setting up cluster $CLUSTER_NAME"
}
create_kind_cluster_or_skip

# 2. Make all the docker images and load them to the kind cluster
VERSION=$VERSION CLUSTER_NAME=$CLUSTER_NAME make push-kind-images

# 3. Build the test helm chart, ensuring we have a chart in the `_test` folder
VERSION=$VERSION make build-test-chart

# 4. Build the gloo command line tool, ensuring we have one in the `_output` folder
make glooctl-linux-amd64

# 5. Install additional resources used for particular KUBE2E tests
if [ "$KUBE2E_TESTS" = "eds" ]; then
  echo "Installing Gloo Edge"
  _output/glooctl-linux-amd64 install gateway --file "_test/gloo-$VERSION".tgz

  kubectl -n gloo-system rollout status deployment gloo --timeout=2m || true
  kubectl -n gloo-system rollout status deployment discovery --timeout=2m || true
  kubectl -n gloo-system rollout status deployment gateway-proxy --timeout=2m || true
  kubectl -n gloo-system rollout status deployment gateway --timeout=2m || true

  echo "Installing Hello World example"
  kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
  _output/glooctl-linux-amd64 add route \
    --path-exact /all-pets \
    --dest-name default-petstore-8080 \
    --prefix-rewrite /api/pets
fi

if [ "$KUBE2E_TESTS" = "glooctl" ]; then
  echo "Installing Istio 1.7.4"
  curl -sSL https://github.com/istio/istio/releases/download/1.7.4/istio-1.7.4-linux-amd64.tar.gz | tar -xzf - istio-1.7.4/bin/istioctl
  ./istio-1.7.4/bin/istioctl install --set profile=minimal
fi
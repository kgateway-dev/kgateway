#!/bin/bash -ex

# 0. Assign default values to some of our environment variables
# Get directory this script is located in to access script local files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
# The name of the kind cluster to deploy to
CLUSTER_NAME="${CLUSTER_NAME:-kind}"
# The version of the Node Docker image to use for booting the cluster
CLUSTER_NODE_VERSION="${CLUSTER_NODE_VERSION:-v1.28.0}"
# The version used to tag images
VERSION="${VERSION:-1.0.0-ci1}"
# Skip building docker images if we are testing a released version
SKIP_DOCKER="${SKIP_DOCKER:-false}"
# Stop after creating the kind cluster
JUST_KIND="${JUST_KIND:-false}"
# Set the default image variant to standard
IMAGE_VARIANT="${IMAGE_VARIANT:-standard}"
# If true, run extra steps to set up k8s gateway api conformance test environment
CONFORMANCE="${CONFORMANCE:-false}"
CILIUM_VERSION="${CILIUM_VERSION:-1.15.5}"

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

  # Install cilium as we need to define custom network policies to simulate kube api server unavailability
  # in some of our kube2e tests
  helm repo add cilium-setup-kind https://helm.cilium.io/
  helm repo update
  helm install cilium cilium-setup-kind/cilium --version $CILIUM_VERSION \
   --namespace kube-system \
   --set image.pullPolicy=IfNotPresent \
   --set ipam.mode=kubernetes \
   --set operator.replicas=1
  helm repo remove cilium-setup-kind
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
  # TODO(tim): refactor the Makefile & CI scripts so we're loading local
  # charts to real helm repos, and then we can remove this block.
  echo "SKIP_DOCKER=true, not building images or chart"
  helm repo add gloo https://storage.googleapis.com/solo-public-helm
  helm repo update
else
  # 2. Make all the docker images and load them to the kind cluster
  VERSION=$VERSION CLUSTER_NAME=$CLUSTER_NAME IMAGE_VARIANT=$IMAGE_VARIANT make kind-build-and-load

  # 3. Build the test helm chart, ensuring we have a chart in the `_test` folder
  VERSION=$VERSION make build-test-chart
fi

# 4. Build the gloo command line tool, ensuring we have one in the `_output` folder
make -s build-cli-local

# 5. Apply the Kubernetes Gateway API CRDs
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml

# 6. Conformance test setup
if [[ $CONFORMANCE == "true" ]]; then
  echo "Running conformance test setup"

  kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml

  # Wait for MetalLB to become available.
  kubectl rollout status -n metallb-system deployment/controller --timeout 2m
  kubectl rollout status -n metallb-system daemonset/speaker --timeout 2m
  kubectl wait -n metallb-system  pod -l app=metallb --for=condition=Ready --timeout=10s

  SUBNET=$(docker network inspect kind | jq -r '.[].IPAM.Config[].Subnet | select(contains(":") | not)' | cut -d '.' -f1,2)
  MIN=${SUBNET}.255.0
  MAX=${SUBNET}.255.231

  # Note: each line below must begin with one tab character; this is to get EOF working within
  # an if block. The `-` in the `<<-EOF`` strips out the leading tab from each line, see
  # https://tldp.org/LDP/abs/html/here-docs.html
	kubectl apply -f - <<-EOF
	apiVersion: metallb.io/v1beta1
	kind: IPAddressPool
	metadata:
	  name: address-pool
	  namespace: metallb-system
	spec:
	  addresses:
	    - ${MIN}-${MAX}
	---
	apiVersion: metallb.io/v1beta1
	kind: L2Advertisement
	metadata:
	  name: advertisement
	  namespace: metallb-system
	spec:
	  ipAddressPools:
	    - address-pool
	EOF
fi

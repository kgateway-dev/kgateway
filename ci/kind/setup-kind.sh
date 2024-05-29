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
# Offer a default value for type of installation
KUBE2E_TESTS="${KUBE2E_TESTS:-gateway}"  # If 'KUBE2E_TESTS' not set or null, use 'gateway'.
# The version of istio to install for glooctl tests. This should get set by the 'setup-kind-cluster' github action, where it is a required input.
ISTIO_VERSION="${ISTIO_VERSION:-1.22.0}"
# Set the default image variant to standard
IMAGE_VARIANT="${IMAGE_VARIANT:-standard}"
# If true, run extra steps to set up k8s gateway api conformance test environment
CONFORMANCE="${CONFORMANCE:-false}"

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

  SUBNET=$(docker network inspect  kind -f '{{(index .IPAM.Config 0).Subnet}}'| cut -d '.' -f1,2)
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

# 7. Install additional resources used for particular KUBE2E tests
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

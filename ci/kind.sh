#!/bin/bash -ex

# This config is roughly based on: https://kind.sigs.k8s.io/docs/user/ingress/
cat <<EOF | kind create cluster --name kind --config=-
kind: Cluster
apiVersion: kind.sigs.k8s.io/v1alpha3
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

# make all the docker images
# write the output to a temp file so that we can grab the image names out of it
# also ensure we clean up the file once we're done
TEMP_FILE=$(mktemp)
VERSION=kind make docker | tee ${TEMP_FILE}

cleanup() {
    echo ">> Removing ${TEMP_FILE}"
    rm ${TEMP_FILE}
}
trap "cleanup" EXIT SIGINT

echo ">> Temporary output file ${TEMP_FILE}"

# grab the image names out of the `make docker` output
sed -nE 's|(\\x1b\[0m)?Successfully tagged (.*$)|\2|p' ${TEMP_FILE} | while read f; do kind load docker-image --name kind $f; done

VERSION=kind make build-test-chart
make glooctl-linux-amd64

if [ "$KUBE2E_TESTS" = "eds" ]; then
  echo "Installing Gloo Edge"
  _output/glooctl-linux-amd64 install gateway --file _test/gloo-kind.tgz

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
  curl -sSL https://github.com/istio/istio/releases/download/1.7.4/istio-1.7.4-linux-amd64.tar.gz | tar -xzf - istio-1.7.4/bin/istioctl
  ./istio-1.7.4/bin/istioctl install --set profile=minimal
fi
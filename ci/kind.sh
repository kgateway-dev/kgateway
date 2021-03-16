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

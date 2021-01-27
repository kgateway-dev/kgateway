#!/bin/bash -ex

# This config is roughly based on: https://kind.sigs.k8s.io/docs/user/ingress/
cat > cluster.yaml << EOF
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
kind create cluster --config=cluster.yaml
rm cluster.yaml

./ci/kind.sh

make glooctl
export PATH=_output:$PATH

if [ "$SETUP" = "discovery" ]; then
  echo "Installing Gloo Edge"
  glooctl install gateway --file _test/gloo-kind.tgz

  kubectl -n gloo-system rollout status deployment gloo --timeout=2m || true
  kubectl -n gloo-system rollout status deployment discovery --timeout=2m || true
  kubectl -n gloo-system rollout status deployment gateway-proxy --timeout=2m || true
  kubectl -n gloo-system rollout status deployment gateway --timeout=2m || true

  echo "Installing Hello World example"
  kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
  glooctl add route \
    --path-exact /all-pets \
    --dest-name default-petstore-8080 \
    --prefix-rewrite /api/pets
fi 
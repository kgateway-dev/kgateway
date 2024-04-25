#!/bin/bash
set -eux

./ci/kind/setup-kind.sh

./projects/gateway2/kind.sh

kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml

helm upgrade --install --create-namespace \
  --namespace gloo-system gloo \
  ./_test/gloo-1.0.0-ci1.tgz \
  -f - <<EOF
discovery:
  enabled: false
EOF

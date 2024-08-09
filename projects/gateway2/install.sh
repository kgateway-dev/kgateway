#!/bin/bash
set -eux

./ci/kind/setup-kind.sh

helm upgrade --install --create-namespace \
  --namespace gloo-system gloo \
  ./_test/gloo-1.0.0-ci1.tgz \
  -f - <<EOF
discovery:
  enabled: false
gateway:
  validation:
    alwaysAcceptResources: false
kubeGateway:
  enabled: true
EOF

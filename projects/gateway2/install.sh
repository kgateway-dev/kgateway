#!/bin/bash
set -eux

./ci/kind/setup-kind.sh

helm upgrade --install --create-namespace \
  --namespace gloo-system gloo \
  ./_test/gloo-1.0.0-ci1.tgz \
  -f ./test/kubernetes/e2e/tests/manifests/k8s-gateway-test-helm.yaml

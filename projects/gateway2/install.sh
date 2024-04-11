#!/bin/bash
set -eux

./ci/kind/setup-kind.sh

./projects/gateway2/kind.sh

helm upgrade --install --create-namespace \
  --namespace gloo-system gloo \
  ./_test/gloo-1.0.0-ci1.tgz \
  -f ./projects/gateway2/tests/conformance/test-values.yaml

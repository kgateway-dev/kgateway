#!/bin/bash
set -eux

./ci/kind/setup-kind.sh

helm upgrade --install --create-namespace \
  --namespace gloo-system gloo \
  ./_test/gloo-1.0.0-ci1.tgz \
  --set kubeGateway.enabled=true \
  --set discovery.enabled=false \
  --set gateway.validation.alwaysAcceptResources=false

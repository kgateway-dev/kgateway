#!/bin/bash
set -eux

./ci/kind/setup-kind.sh

helm upgrade -i -n gloo-system gloo ./_test/gloo-1.0.0-ci1.tgz --create-namespace --set kubeGateway.enabled=true

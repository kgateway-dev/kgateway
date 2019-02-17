#!/usr/bin/env bash

set -e

cd $GLOO_DIR

rm -rf _output
export VERSION=dev

# Default namespace to gloo-system unless specified
namespace="gloo-system"
if [[ -n $1 ]]; then
    namespace=$1
fi

./hack/kube/safe_delete_namespace.sh $namespace

eval $(minikube docker-env)
make docker -B
make manifest -B INSTALL_NAMESPACE=$namespace

kubectl create namespace $namespace

kubectl config set-context $(kubectl config current-context) --namespace=$namespace

kubectl apply -n $namespace -f $GLOO_DIR/install/gloo-gateway.yaml

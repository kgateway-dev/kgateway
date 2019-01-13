#!/bin/bash -ex

CLUSTER_NAME="test"

function finish {
    unset KUBECONFIG
    kind delete cluster --name $CLUSTER_NAME
}

export RUN_KUBE_TESTS=1

# deploy cluster
kind create cluster --name=$CLUSTER_NAME --wait=2m --image=kindest/node:v1.11.3
trap finish EXIT ERR

# init kubectl
export KUBECONFIG="$(kind get kubeconfig-path --name="$CLUSTER_NAME")"

# get the container id of the cluster container
export KIND_CONTAINER_ID=$(docker ps |grep kindest/node |grep $CLUSTER_NAME | cut -f1 -d' ')

# test!
ginkgo -r -failFast
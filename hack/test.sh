#!/bin/bash -e

export RUN_KUBE2E_TESTS=1
export RUN_KUBE_TESTS=1
export DEBUG=1

function cleanup {
    unset KUBECONFIG
    kind delete cluster --name $CLUSTER_NAME
}

function start {
    kind create cluster --name=$CLUSTER_NAME --wait=2m --image=kindest/node:v1.28.0
}

function kind-env {
    export CLUSTER_NAME=${CLUSTER_NAME:-"test"}
    echo export CLUSTER_NAME=$CLUSTER_NAME
    echo export KUBECONFIG="$(kind get kubeconfig-path --name="$CLUSTER_NAME")"
    echo export KIND_CONTAINER_ID=$(docker ps |grep kindest/node |grep $CLUSTER_NAME | cut -f1 -d' ')
    echo "# To use, run:"
    echo "# eval \$($0 kind-env)"
}

function runtests {
    ginkgo -failFast -v -r
}

function testall {
    start
    trap cleanup EXIT ERR
    export DEBUG=1
    runtests
}

eval $(kind-env)

case "$1" in
"kind-env")
    kind-env
    ;;
"start")
    start
    ;;
"cleanup")
    cleanup
    ;;
"testall")
    testall
    ;;
"runtests")
    runtests
    ;;
*)
    echo "choose one of: kind-env,start,cleanup,runtests,testall"
    echo "After the first time you run runtests, consider setting SKIP_BUILD=1"
    echo "If you want the test e2e to stop on failure, WAIT_ON_FAIL=1"
    ;;
esac

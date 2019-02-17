#!/usr/bin/env bash

set -e

# Default namespace to gloo-system unless specified
namespace="gloo-system"
if [[ -n $1 ]]; then
    namespace=$1
fi

context=$(kubectl config current-context)
if [[ "$context" != "minikube" ]]; then
    echo "current context is set to $context, unable to delete context other than minikube"
    echo "if this is your intention run the following command manually:"
    echo -e "\n$ kubectl delete namespace $namespace\n"
    echo "To switch context back to minikube run the following:"
    echo -e "\n$ kubectl config use-context minikube \n"
    exit 1
fi

kubectl delete namespace $namespace || true


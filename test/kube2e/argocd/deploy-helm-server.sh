#!/usr/bin/env bash
set -e

kubectl apply -f helm-repo.yaml
kubectl rollout status deployment helm-repo
POD=$(kubectl get pods --no-headers -o custom-columns=":metadata.name" | grep "helm-repo")
for file in ../../../_test/*; do
    echo $file
    kubectl cp $file $POD:/usr/share/nginx/html/
done
kubectl cp helm-override.yaml $POD:/usr/share/nginx/html/


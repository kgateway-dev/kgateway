#!/bin/bash

go get -u github.com/onsi/ginkgo/ginkgo
make update-deps

# make all the docker images
# write the output to a temp file so that we can grab the image names out of it
# also ensure we clean up the file once we're done
TEMP_FILE=$(mktemp)
make docker | tee ${TEMP_FILE}

cleanup() {
    echo ">> Removing ${TEMP_FILE}"
    rm ${TEMP_FILE}
}
trap "cleanup" EXIT SIGINT

echo ">> Temporary output file ${TEMP_FILE}"

# grab the image names out of the `make docker` output
sed -nE 's|Successfully tagged (.*$)|\1|p' ${TEMP_FILE} | while read f; do kind load docker-image --name kind $f; done


make build-kind-chart

kubectl create namespace gloo-system || true

make deploy-kind-chart

kubectl -n gloo-system rollout status deployment gloo --timeout=1m
kubectl -n gloo-system rollout status deployment gateway --timeout=1m
kubectl -n gloo-system rollout status deployment gateway-proxy --timeout=1m
kubectl -n gloo-system rollout status deployment discovery --timeout=1m

#!/bin/bash

# make all the docker images
# write the output to a temp file so that we can grab the image names out of it
# also ensure we clean up the file once we're done
TEMP_FILE=$(mktemp)
VERSION=kind make docker | tee ${TEMP_FILE}

cleanup() {
    echo ">> Removing ${TEMP_FILE}"
    rm ${TEMP_FILE}
}
trap "cleanup" EXIT SIGINT

echo ">> Temporary output file ${TEMP_FILE}"

# grab the image names out of the `make docker` output
sed -nE 's|(\\x1b\[0m)?Successfully tagged (.*$)|\2|p' ${TEMP_FILE} | while read f; do kind load docker-image --name kind $f; done

VERSION=kind make build-test-chart
make glooctl-linux-amd64

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
sed -nE 's|Successfully tagged (.*$)|\1|p' ${TEMP_FILE} | while read f; do kind load docker-image --name kind $f; done

# This is just for a time optimization, so that we aren't pulling the testrunner image during the test
docker pull soloio/testrunner:latest
kind load docker-image soloio/testrunner

VERSION=kind make build-kind-chart
make glooctl-linux-amd64

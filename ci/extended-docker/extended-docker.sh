#!/bin/bash

# make all the docker images
# write the output to a temp file so that we can grab the image names out of it
# also ensure we clean up the file once we're done
TEMP_FILE=$(mktemp)
make docker-push | tee ${TEMP_FILE}

cleanup() {
    echo ">> Removing ${TEMP_FILE}"
    rm ${TEMP_FILE}
}
trap "cleanup" EXIT SIGINT

echo ">> Temporary output file ${TEMP_FILE}"

# grab the image names out of the `make docker` output
sed -nE 's|Successfully tagged (.*$)|\1|p' ${TEMP_FILE} | while read f;
do
  docker build ci/extended-docker --build-arg BASE_IMAGE=$f -t $f-extended;
  docker push $f-extended;
done
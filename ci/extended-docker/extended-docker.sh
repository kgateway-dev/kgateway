#!/bin/bash

# make all the docker images
# write the output to a temp file so that we can grab the image names out of it
# also ensure we clean up the file once we're done
TEMP_FILE=$(mktemp)
make docker-push | tee ${TEMP_FILE}
err=${PIPESTATUS[0]}

cleanup() {
    echo ">> Removing ${TEMP_FILE}"
    rm ${TEMP_FILE}
}
trap "cleanup" EXIT SIGINT

echo ">> Temporary output file ${TEMP_FILE}"
if [ "$err" != 0 ]; then
  exit $err
fi
# grab the image names out of the `make docker` output
sed -nE 's|(^.*(\\x1b\[0m)?.*)[-]t ([^ \\]*).*|\3|p' ${TEMP_FILE} | grep -v 'build-container' | grep -v '[-]race' | while read f;
do
  docker build ci/extended-docker --build-arg BASE_IMAGE=$f -t $f-extended;
  docker push $f-extended;
done

#!/bin/bash -ex

CI_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
DIR="$( cd "$CI_DIR/.." >/dev/null && pwd )"

[[ -z "${OUTPUT_DIR}" ]] && OUTPUT_DIR="$( cd "$DIR/../_output" >/dev/null && pwd )"

cp -f $DIR/WORKSPACE $CI_DIR/WORKSPACE
cp -f $DIR/BUILD $CI_DIR/BUILD

# replace ../ with /source/
sed -e 's$\.\./$/source/$' -i $CI_DIR/WORKSPACE
# replace ./ with /source/build-envoy
sed -e 's$\./$/source/build-envoy/$' -i $CI_DIR/WORKSPACE
# make envoy dependencies use prebuilt for faster build
sed -e 's$envoy_dependencies()$envoy_dependencies(path = "@envoy//ci/prebuilt")$' -i $CI_DIR/WORKSPACE

# cd to workspace
cd $CI_DIR

ENVOY_SRCDIR="$DIR/envoy"
. $DIR/envoy/ci/build_setup.sh -nofetch

export BAZEL_OPTS="-c opt"

$DIR/run_tests.sh
bazel --batch build $BAZEL_BUILD_OPTIONS $BAZEL_OPTS //:envoy

# copy and strip envoy for a small binary
cp -f \
    "${CI_DIR}"/bazel-bin/envoy \
    "${OUTPUT_DIR}"/envoy
strip "${OUTPUT_DIR}"/envoy -o "${OUTPUT_DIR}"/envoy-stripped

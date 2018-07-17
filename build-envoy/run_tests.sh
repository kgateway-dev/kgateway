#!/bin/bash -ex

[ -z "${BAZEL_OPTS}" ] && export BAZEL_OPTS="-c dbg"


bazel test $BAZEL_TEST_OPTIONS $BAZEL_OPTS \
    @transformation_filter//test/... \
    @solo_envoy_common//test/... \
    @aws_lambda//test/... \
    @nats_streaming_filter//test/... \
    @azure_functions_filter//test/... \
    @consul_connect_filter//test/... \
    @google_functions//test/...

bazel test  -c dbg \
    @transformation_filter//test/... \
    @solo_envoy_common//test/... \
    @aws_lambda//test/... \
    @nats_streaming_filter//test/... \
    @azure_functions_filter//test/... \
    @google_functions//test/...
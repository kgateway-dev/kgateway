licenses(["notice"])  # Apache 2

load(
    "@envoy//bazel:envoy_build_system.bzl",
    "envoy_cc_binary",
    "envoy_package",
)

envoy_package()


# TODO(yuval-k): allow a non stamped version till we figure out why stamping doesn't work..

envoy_cc_binary(
    name = "envoy",
    stamped = False,
    repository = "@envoy",
    deps = [
        "@envoy//source/exe:envoy_main_entry_lib",
    ],
)

#!/bin/sh
set -eu

if "${DISABLE_CORE_DUMPS:-false}" ; then
  ulimit -c 0
fi

if [ -n "${ENVOY_SIDECAR:-}" ] # true if ENVOY_SIDECAR is a non-empty string
then
  /usr/local/bin/envoy -c /etc/envoy/envoy-sidecar.yaml "$@"
else
  /usr/local/bin/envoyinit "$@"
fi

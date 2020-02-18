#!/bin/sh
set -e

if [ -n "$ENVOY_SIDECAR" ]
then
  exec /usr/local/bin/envoy -c /etc/envoy/envoy-sidecar.yaml "$@"
else
  exec /usr/local/bin/envoyinit "$@"
fi
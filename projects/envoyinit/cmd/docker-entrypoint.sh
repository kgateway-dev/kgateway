#!/bin/sh
set -eu

if $DISABLE_CORE_DUMPS ; then
  ulimit -c 0
fi

if [ -n "${ENVOY_SIDECAR:-}" ] # true if ENVOY_SIDECAR is a non-empty string
then
  # FIXME delete this
  while sleep 2 ; do
    /usr/local/bin/envoy -c /etc/envoy/envoy-sidecar.yaml "$@"
  done
else
  # FIXME delete this
  while sleep 2 ; do
    /usr/local/bin/envoyinit "$@"
  done
fi

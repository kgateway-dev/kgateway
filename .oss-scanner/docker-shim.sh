#!/bin/sh
# Stands in for `docker` in the OSS Scanner image, which has neither Docker nor a network. Tests that validate Envoy
# config call pkg/validator's docker validator, which runs
#   docker run --rm -i [--pull P] [-v DIR:/etc/envoy/:ro] --entrypoint /usr/local/bin/envoy IMAGE ENVOY_ARGS...
# This runs the same Envoy binary locally instead, with DIR linked at /etc/envoy.
set -eu
[ "${1:-}" = run ] || { echo "docker shim: only 'docker run' is supported" >&2; exit 1; }
shift
entrypoint=
while [ $# -gt 0 ]; do
  case "$1" in
    --rm | -i) shift ;;
    --pull) shift 2 ;;
    -v)
      src=${2%%:*}
      ln -sfn "$src" /etc/envoy
      shift 2
      ;;
    --entrypoint) entrypoint=$2; shift 2 ;;
    -*) echo "docker shim: unsupported flag $1" >&2; exit 1 ;;
    *) shift; break ;; # the image
  esac
done
# The envoy-wrapper image sets this so Envoy finds librust_module.so.
export ENVOY_DYNAMIC_MODULES_SEARCH_PATH=/usr/local/lib
exec "${entrypoint:-/usr/local/bin/envoy}" "$@"

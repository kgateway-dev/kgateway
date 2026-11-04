ARG ENVOY_IMAGE
ARG BASE_IMAGE=cgr.dev/chainguard/glibc-dynamic:latest

FROM ${ENVOY_IMAGE} AS envoy

FROM busybox:1.36.1-glibc AS busybox

FROM ${BASE_IMAGE}

ARG GOARCH=amd64

COPY --from=busybox /bin/busybox /bin/sh
COPY --from=busybox /bin/busybox /usr/bin/wget
COPY --from=envoy /usr/local/bin/envoy /usr/local/bin/envoy
COPY --from=envoy /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY envoyinit-linux-$GOARCH /usr/local/bin/envoyinit

ENV ENVOY_DYNAMIC_MODULES_SEARCH_PATH=/usr/local/lib
ARG RUST_MODULE_DIR=.
COPY ${RUST_MODULE_DIR}/librust_module.so /usr/local/lib/librust_module.so

# SDS-specific setup, only used if ENVOY_SIDECAR=true
ARG ENTRYPOINT_SCRIPT=/docker-entrypoint.sh
COPY $ENTRYPOINT_SCRIPT /

USER 10101

ENTRYPOINT ["/bin/sh", "/docker-entrypoint.sh"]
CMD []

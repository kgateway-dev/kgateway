ARG ENVOY_IMAGE
ARG BASE_IMAGE=cgr.dev/chainguard/glibc-dynamic:latest

# Stage 1: source of the envoy binary used by the in-process xDS validator
# (validator.NewBinary shells out to `envoy --mode validate`).
FROM $ENVOY_IMAGE AS envoy

# Stage 2: minimal distroless runtime. The Chainguard glibc base ships CA certs,
# libgcc_s, and libstdc++, which the envoy binary and the rust dynamic module need.
FROM $BASE_IMAGE

ARG GOARCH=amd64

COPY --from=envoy /usr/local/bin/envoy /usr/local/bin/envoy

COPY kgateway-linux-$GOARCH /usr/local/bin/kgateway

ENV ENVOY_DYNAMIC_MODULES_SEARCH_PATH=/usr/local/lib
ARG RUST_MODULE_DIR=.
COPY ${RUST_MODULE_DIR}/librust_module.so /usr/local/lib/librust_module.so

USER 10101

ENTRYPOINT ["/usr/local/bin/kgateway"]

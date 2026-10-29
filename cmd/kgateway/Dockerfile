ARG ENVOY_IMAGE
ARG BASE_IMAGE=cgr.dev/chainguard/glibc-dynamic:latest

# Stage 1: source of the envoy binary used by the in-process xDS validator
# (validator.NewBinary shells out to `envoy --mode validate`).
FROM $ENVOY_IMAGE AS envoy

# Stage 2: minimal distroless runtime. We use the Chainguard glibc-dynamic base
# because the envoy binary is dynamically linked against glibc (libc, libm,
# libpthread, libdl, librt). The base also ships CA certificates for the control
# plane's outbound TLS. envoy statically links libc++ and libgcc, and the kgateway
# Go binary is built CGO_ENABLED=0 (fully static), so the runtime does not need libstdc++.
FROM $BASE_IMAGE

ARG GOARCH=amd64

COPY --from=envoy /usr/local/bin/envoy /usr/local/bin/envoy

COPY kgateway-linux-$GOARCH /usr/local/bin/kgateway

USER 10101

ENTRYPOINT ["/usr/local/bin/kgateway"]

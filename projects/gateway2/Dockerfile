# syntax=docker/dockerfile:1
FROM gcr.io/distroless/base-debian11 

WORKDIR / 

COPY gloo-gateway /usr/local/bin/gloo-gateway

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/gloo-gateway"]

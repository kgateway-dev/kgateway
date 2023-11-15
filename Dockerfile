# syntax=docker/dockerfile:1
FROM gcr.io/distroless/base-debian11 

ARG GOARCH=amd64

WORKDIR / 

COPY gloo-gateway-linux-$GOARCH /usr/local/bin/gloo-gateway

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/gloo-gateway"]

# syntax=docker/dockerfile:1
FROM gcr.io/distroless/base-debian11 

WORKDIR / 

COPY gloo-gateway2 /usr/local/bin/gloo-gateway2

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/gloo-gateway2"]

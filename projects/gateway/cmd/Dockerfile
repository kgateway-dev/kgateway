FROM alpine:3.11.3

ARG GOARCH=amd64

COPY gateway-linux-$GOARCH /usr/local/bin/gateway

USER 10101

ENTRYPOINT ["/usr/local/bin/gateway"]
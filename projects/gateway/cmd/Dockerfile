FROM alpine:3.12.1

ARG GOARCH=amd64

COPY gateway-linux-$GOARCH /usr/local/bin/gateway

USER 10101

ENTRYPOINT ["/usr/local/bin/gateway"]
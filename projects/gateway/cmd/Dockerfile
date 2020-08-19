ARG GOARCH=amd64

FROM alpine:3.11.3

COPY gateway-linux-$GOARCH /usr/local/bin/gateway

ENTRYPOINT ["/usr/local/bin/gateway"]

USER 10101
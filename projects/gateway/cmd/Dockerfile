FROM alpine:3.15.4

ARG GOARCH=amd64

RUN apk -U upgrade

COPY gateway-linux-$GOARCH /usr/local/bin/gateway

USER 10101

ENTRYPOINT ["/usr/local/bin/gateway"]
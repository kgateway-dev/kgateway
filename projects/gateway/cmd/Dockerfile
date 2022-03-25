FROM alpine:3.13.8

ARG GOARCH=amd64

RUN apk -U upgrade

COPY gateway-linux-$GOARCH /usr/local/bin/gateway

USER 10101

ENTRYPOINT ["/usr/local/bin/gateway"]
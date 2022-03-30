FROM alpine:3.13.8

ARG GOARCH=amd64

RUN apk -U upgrade

COPY sds-linux-$GOARCH /usr/local/bin/sds

USER 10101

ENTRYPOINT ["/usr/local/bin/sds"]

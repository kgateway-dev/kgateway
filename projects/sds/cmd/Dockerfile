FROM alpine:3.12.1.3

ARG GOARCH=amd64

COPY sds-linux-$GOARCH /usr/local/bin/sds

USER 10101

ENTRYPOINT ["/usr/local/bin/sds"]

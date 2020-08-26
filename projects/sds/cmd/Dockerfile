FROM alpine:3.11.3

ARG GOARCH=amd64

COPY sds-linux-$GOARCH /usr/local/bin/sds

ENTRYPOINT ["/usr/local/bin/sds"]

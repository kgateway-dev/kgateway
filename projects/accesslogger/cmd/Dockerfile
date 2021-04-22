FROM alpine:3.13.4

ARG GOARCH=amd64

RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*
COPY access-logger-linux-$GOARCH /usr/local/bin/access-logger

USER 10101

ENTRYPOINT ["/usr/local/bin/access-logger"]
FROM alpine:3.11.3

ARG GOARCH=amd64

RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*
COPY access-logger-linux-$GOARCH /usr/local/bin/access-logger

ENTRYPOINT ["/usr/local/bin/access-logger"]
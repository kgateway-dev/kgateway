FROM alpine:3.11.6

RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*
COPY access-logger-linux-amd64 /usr/local/bin/access-logger

ENTRYPOINT ["/usr/local/bin/access-logger"]
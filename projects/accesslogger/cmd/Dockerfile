FROM alpine:3.13.5

ARG GOARCH=amd64
ARG PACKAGE_UPDATES
RUN apk update &&apk add $PACKAGE_UPDATES ca-certificates&&rm -rf /var/cache/apk/*
COPY access-logger-linux-$GOARCH /usr/local/bin/access-logger

USER 10101

ENTRYPOINT ["/usr/local/bin/access-logger"]
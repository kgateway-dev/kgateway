FROM alpine:3.15.4

RUN apk upgrade --update-cache \
    && apk update \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY sleeper-linux-amd64 /usr/local/bin/sleeper

ENTRYPOINT ["/usr/local/bin/sleeper"]
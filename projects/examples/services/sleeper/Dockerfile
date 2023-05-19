FROM alpine:3.17.3

RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY sleeper-linux-amd64 /usr/local/bin/sleeper

ENTRYPOINT ["/usr/local/bin/sleeper"]
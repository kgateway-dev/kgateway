FROM alpine:3.17.6

RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY app-linux-amd64 /usr/local/bin/app

ENTRYPOINT ["/usr/local/bin/app"]

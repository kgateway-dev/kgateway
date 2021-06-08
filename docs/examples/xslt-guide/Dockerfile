FROM alpine:3.13.4

RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY server-linux-amd64 /usr/local/bin/app
COPY pkg/world_cities.csv /usr/local/bin/world_cities.csv

ENTRYPOINT ["/usr/local/bin/app"]

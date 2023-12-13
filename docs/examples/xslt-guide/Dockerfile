FROM alpine:3.17.3

RUN apk upgrade --update-cache \
    && apk add ca-certificates curl \
    && rm -rf /var/cache/apk/*

COPY server-linux-amd64 /usr/local/bin/app

RUN curl https://raw.githubusercontent.com/datasets/world-cities/master/data/world-cities.csv | tail -n +2 > /usr/local/bin/world_cities.csv
ENTRYPOINT ["/usr/local/bin/app"]

ARG GOARCH=amd64

FROM alpine:3.11.3

# Needed for access to AWS
RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY discovery-linux-$GOARCH /usr/local/bin/discovery

ENTRYPOINT ["/usr/local/bin/discovery"]
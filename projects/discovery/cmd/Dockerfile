FROM alpine:3.11.6

# Needed for access to AWS
RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY discovery-linux-amd64 /usr/local/bin/discovery

ENTRYPOINT ["/usr/local/bin/discovery"]
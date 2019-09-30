FROM alpine

# Needed for access to AWS
RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY certgen-linux-amd64 /usr/local/bin/certgen

ENTRYPOINT ["/usr/local/bin/certgen"]
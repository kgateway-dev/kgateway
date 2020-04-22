ARG ENVOY_IMAGE
FROM $ENVOY_IMAGE

RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY gloo-linux-amd64 /usr/local/bin/gloo
COPY third_party_licenses.txt /etc/third_party_licenses.txt

ENTRYPOINT ["/usr/local/bin/gloo"]
FROM quay.io/solo-io/envoy-gloo:1.4.2

RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY gloo-linux-amd64 /usr/local/bin/gloo
COPY third_party_licenses.txt /etc/third_party_licenses.txt

ENTRYPOINT ["/usr/local/bin/gloo"]
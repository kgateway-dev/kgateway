ARG ENVOY_IMAGE

FROM $ENVOY_IMAGE

ARG GOARCH=amd64

RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY gloo-linux-$GOARCH /usr/local/bin/gloo
COPY third_party_licenses.txt /etc/third_party_licenses.txt

USER 10101

ENTRYPOINT ["/usr/local/bin/gloo"]
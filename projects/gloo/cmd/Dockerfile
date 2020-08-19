ARG GOARCH=amd64
ARG ENVOY_IMAGE

FROM $ENVOY_IMAGE

RUN apk upgrade --update-cache \
    && apk add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY gloo-linux-$GOARCH /usr/local/bin/gloo
COPY third_party_licenses.txt /etc/third_party_licenses.txt

ENTRYPOINT ["/usr/local/bin/gloo"]
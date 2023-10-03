ARG ENVOY_IMAGE

FROM $ENVOY_IMAGE

ARG GOARCH=amd64
# eventually may matter for now https://unix.stackexchange.com/a/701288
# means its not too useful
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install --no-install-recommends -y  ca-certificates \
    && rm -rf  /var/log/*log /var/lib/apt/lists/* /var/log/apt/* /var/lib/dpkg/*-old /var/cache/debconf/*-old


COPY gloo-linux-$GOARCH /usr/local/bin/gloo

USER 10101

ENTRYPOINT ["/usr/local/bin/gloo"]
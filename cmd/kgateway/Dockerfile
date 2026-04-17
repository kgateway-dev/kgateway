ARG ENVOY_IMAGE

FROM $ENVOY_IMAGE

ARG GOARCH=amd64
# eventually may matter for now https://unix.stackexchange.com/a/701288
# means its not too useful
ENV DEBIAN_FRONTEND=noninteractive

# Update our deps to make cve toil lower
#install wget for our default probes
RUN (sed -i 's|http://archive.ubuntu.com|http://azure.archive.ubuntu.com|g; s|http://security.ubuntu.com|http://azure.archive.ubuntu.com|g' /etc/apt/sources.list 2>/dev/null || true) \
    && (sed -i 's|http://archive.ubuntu.com|http://azure.archive.ubuntu.com|g; s|http://security.ubuntu.com|http://azure.archive.ubuntu.com|g' /etc/apt/sources.list.d/ubuntu.sources 2>/dev/null || true) \
    && apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 upgrade -y \
    && apt-get -o Acquire::Retries=5 install --no-install-recommends -y ca-certificates wget \
    && rm -rf  /var/log/*log /var/lib/apt/lists/* /var/log/apt/* /var/lib/dpkg/*-old /var/cache/debconf/*-old

COPY kgateway-linux-$GOARCH /usr/local/bin/kgateway

USER 10101

ENTRYPOINT ["/usr/local/bin/kgateway"]

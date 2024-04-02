ARG PACKAGE_DONOR_IMAGE
ARG BASE_IMAGE

FROM $PACKAGE_DONOR_IMAGE as donor
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install --no-install-recommends -y ca-certificates

FROM $BASE_IMAGE
# Copy over the certs
COPY --from=donor /usr/share/ca-certificates /usr/share/ca-certificates
COPY --from=donor /usr/share/doc/ca-certificates/ /usr/share/doc/ca-certificates/
COPY --from=donor /etc/ssl /etc/ssl
COPY --from=donor /usr/lib/ssl /usr/lib/ssl

FROM bitnami/kubectl:1.22.12 as kubectl

FROM alpine:3.17.3

# Fix for CVE-2023-2650. If alpine upgrades this dep, we can remove this line.
RUN apk add openssl=3.0.9-r1

COPY --from=kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/

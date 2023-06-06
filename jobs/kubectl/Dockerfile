FROM bitnami/kubectl:1.26.4 as kubectl

FROM alpine:3.17.3

# fix for CVE-2023-2650
RUN apk add openssl=3.0.9-r1

COPY --from=kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/

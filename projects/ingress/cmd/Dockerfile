FROM alpine:3.17.3

ARG GOARCH=amd64

RUN apk -U upgrade

# Fix for CVE-2023-2650. If alpine upgrades this dep, we can remove this line.
RUN apk add openssl=3.0.9-r1

COPY ingress-linux-$GOARCH /usr/local/bin/ingress

USER 10101

ENTRYPOINT ["/usr/local/bin/ingress"]
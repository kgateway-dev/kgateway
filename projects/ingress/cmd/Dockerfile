FROM alpine:3.13.2

ARG GOARCH=amd64

COPY ingress-linux-$GOARCH /usr/local/bin/ingress

USER 10101

ENTRYPOINT ["/usr/local/bin/ingress"]
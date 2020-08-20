FROM alpine:3.11.3

ARG GOARCH=amd64

COPY ingress-linux-$GOARCH /usr/local/bin/ingress

ENTRYPOINT ["/usr/local/bin/ingress"]
ARG GOARCH=amd64

FROM alpine:3.11.3

COPY ingress-linux-$GOARCH /usr/local/bin/ingress

ENTRYPOINT ["/usr/local/bin/ingress"]
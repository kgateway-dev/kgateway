<<<<<<< HEAD
FROM alpine:3.11.3
=======
FROM alpine:3.13.5
>>>>>>> eecee1562... bump alpine and jwt-go (#4723) (#4725)

ARG GOARCH=amd64

COPY ingress-linux-$GOARCH /usr/local/bin/ingress

USER 10101

ENTRYPOINT ["/usr/local/bin/ingress"]
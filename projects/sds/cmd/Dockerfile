<<<<<<< HEAD
FROM alpine:3.11.3
=======
FROM alpine:3.13.5
>>>>>>> eecee1562... bump alpine and jwt-go (#4723) (#4725)

ARG GOARCH=amd64

COPY sds-linux-$GOARCH /usr/local/bin/sds

USER 10101

ENTRYPOINT ["/usr/local/bin/sds"]

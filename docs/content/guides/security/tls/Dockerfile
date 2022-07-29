FROM alpine:3.15.5

COPY cert.pem /cert.pem
COPY key.pem /key.pem
COPY example-tls-server /usr/local/bin/example-tls-server

ENTRYPOINT ["/usr/local/bin/example-tls-server"]

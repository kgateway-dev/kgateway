FROM cgr.dev/chainguard/static:latest

ARG GOARCH=amd64

COPY dummy-idp-linux-$GOARCH /usr/local/bin/dummy-idp

USER 10101

ENTRYPOINT ["/usr/local/bin/dummy-idp"]

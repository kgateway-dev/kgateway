FROM cgr.dev/chainguard/static:latest

ARG GOARCH=amd64

COPY extauth-server-linux-$GOARCH /server

USER nonroot:nonroot

EXPOSE 9000
ENTRYPOINT ["/server"]

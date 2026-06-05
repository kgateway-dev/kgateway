FROM cgr.dev/chainguard/static

ARG GOARCH=amd64

COPY extproc-server-linux-$GOARCH /server

USER nonroot:nonroot

EXPOSE 18080
CMD ["/server"]

ARG BASE_IMAGE=cgr.dev/chainguard/static:latest

FROM $BASE_IMAGE

ARG GOARCH=amd64

COPY extproc-server-linux-$GOARCH /server

USER nonroot:nonroot

EXPOSE 18080
CMD ["/server"]

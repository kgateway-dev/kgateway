ARG BASE_IMAGE=cgr.dev/chainguard/glibc-dynamic:latest

FROM $BASE_IMAGE

ARG GOARCH=amd64

COPY sds-linux-$GOARCH /usr/local/bin/sds

USER 10101

ENTRYPOINT ["/usr/local/bin/sds"]

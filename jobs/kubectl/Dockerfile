FROM bitnami/kubectl:1.22.12 as kubectl

FROM alpine:3.17.3

COPY --from=kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/

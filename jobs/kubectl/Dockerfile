FROM bitnami/kubectl:1.24.7 as kubectl

FROM alpine:3.17.2

COPY --from=kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/

FROM bitnami/kubectl:1.25.8 as kubectl

FROM alpine:3.17.2

COPY --from=kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/

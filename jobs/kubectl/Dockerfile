FROM alpine:3.15.5
FROM bitnami/kubectl:1.22.12 as kubectl

COPY --from=kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/

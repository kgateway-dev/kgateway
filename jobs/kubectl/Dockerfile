FROM bitnami/kubectl:1.25.3 as kubectl

FROM alpine:3.17.3

COPY --from=kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/

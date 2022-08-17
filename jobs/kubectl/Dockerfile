FROM bitnami/kubectl:1.22.12 as kubectl

FROM alpine:3.15.6

COPY --from=kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/
